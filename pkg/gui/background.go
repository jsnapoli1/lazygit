package gui

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/jesseduffield/gocui"
	"github.com/jesseduffield/lazygit/pkg/gui/types"
	"github.com/jesseduffield/lazygit/pkg/utils"
)

type BackgroundRoutineMgr struct {
	gui *Gui

	// if we've suspended the gui (e.g. because we've switched to a subprocess)
	// we typically want to pause some things that are running like background
	// file refreshes
	pauseBackgroundRefreshes bool

	// a channel to trigger an immediate background fetch; we use this when switching repos
	triggerFetch chan struct{}

	// tracks consecutive fetch failures for exponential backoff
	consecutiveFailures int

	// filesystem event watcher for event-driven file refresh
	fileWatcher *FileWatcher
}

func (self *BackgroundRoutineMgr) PauseBackgroundRefreshes(pause bool) {
	self.pauseBackgroundRefreshes = pause
}

func (self *BackgroundRoutineMgr) startBackgroundRoutines() {
	userConfig := self.gui.UserConfig()

	if userConfig.Git.AutoFetch {
		fetchInterval := userConfig.Refresher.FetchInterval
		if fetchInterval > 0 {
			go utils.Safe(self.startBackgroundFetch)
		} else {
			self.gui.c.Log.Errorf(
				"Value of config option 'refresher.fetchInterval' (%d) is invalid, disabling auto-fetch",
				fetchInterval)
		}
	}

	if userConfig.Git.AutoRefresh {
		refreshInterval := userConfig.Refresher.RefreshInterval
		if refreshInterval > 0 {
			self.startFileWatcher()
			go utils.Safe(self.startBackgroundFilesRefresh)
		} else {
			self.gui.c.Log.Errorf(
				"Value of config option 'refresher.refreshInterval' (%d) is invalid, disabling auto-refresh",
				refreshInterval)
		}
	}

	if self.gui.Config.GetDebug() {
		self.goEvery(time.Second*time.Duration(10), 0, self.gui.stopChan, func() error {
			formatBytes := func(b uint64) string {
				const unit = 1000
				if b < unit {
					return fmt.Sprintf("%d B", b)
				}
				div, exp := uint64(unit), 0
				for n := b / unit; n >= unit; n /= unit {
					div *= unit
					exp++
				}
				return fmt.Sprintf("%.1f %cB",
					float64(b)/float64(div), "kMGTPE"[exp])
			}

			m := runtime.MemStats{}
			runtime.ReadMemStats(&m)
			self.gui.c.Log.Infof("Heap memory in use: %s", formatBytes(m.HeapAlloc))
			return nil
		})
	}
}

func (self *BackgroundRoutineMgr) startBackgroundFetch() {
	self.gui.waitForIntro.Wait()

	fetch := func() error {
		// Pre-check: attempt TCP dial to remote before fetching.
		// This avoids hanging for the OS-level TCP timeout when the remote is unreachable.
		if err := self.gui.git.Sync.CheckReachability("origin", 2*time.Second); err != nil {
			self.gui.c.Log.Infof("Background fetch: remote unreachable, skipping: %v", err)
			self.consecutiveFailures++
			return nil
		}

		// Do this on the UI thread so that we don't have to deal with synchronization around the
		// access of the repo state.
		self.gui.onUIThread(func() error {
			// There's a race here, where we might be recording the time stamp for a different repo
			// than where the fetch actually ran. It's not very likely though, and not harmful if it
			// does happen; guarding against it would be more effort than it's worth.
			self.gui.State.LastBackgroundFetchTime = time.Now()
			return nil
		})

		err := self.gui.helpers.AppStatus.WithWaitingStatusImpl(self.gui.Tr.FetchingStatus, func(gocui.Task) error {
			return self.backgroundFetch()
		}, nil)

		if err != nil {
			self.gui.c.Log.Infof("Background fetch failed: %v", err)
			self.consecutiveFailures++
		} else {
			self.consecutiveFailures = 0
		}
		return err
	}

	// We want an immediate fetch at startup, and since goEvery starts by
	// waiting for the interval, we need to trigger one manually first
	_ = fetch()

	userConfig := self.gui.UserConfig()
	self.triggerFetch = self.goEvery(userConfig.Refresher.FetchIntervalDuration(), 8, self.gui.stopChan, fetch)
}

func (self *BackgroundRoutineMgr) startFileWatcher() {
	repoPaths := self.gui.git.RepoPaths
	watcher, err := NewFileWatcher(
		self.gui.c.Log,
		func(opts types.RefreshOptions) { self.gui.c.Refresh(opts) },
		func() bool { return self.pauseBackgroundRefreshes },
		repoPaths.WorktreePath(),
		repoPaths.WorktreeGitDirPath(),
		repoPaths.RepoGitDirPath(),
	)
	if err != nil {
		self.gui.c.Log.Warnf("Failed to create file watcher, falling back to polling: %v", err)
		return
	}

	if err := watcher.Start(); err != nil {
		self.gui.c.Log.Warnf("Failed to start file watcher, falling back to polling: %v", err)
		watcher.Close()
		return
	}

	self.fileWatcher = watcher
	self.gui.c.Log.Infof("File watcher started successfully")
}

func (self *BackgroundRoutineMgr) startBackgroundFilesRefresh() {
	self.gui.waitForIntro.Wait()

	userConfig := self.gui.UserConfig()
	interval := userConfig.Refresher.RefreshIntervalDuration()

	// When the file watcher is active, polling is just a safety net;
	// use a much longer interval to reduce unnecessary work
	if self.fileWatcher != nil {
		const fallbackInterval = 5 * time.Minute
		if interval < fallbackInterval {
			interval = fallbackInterval
		}
	}

	self.goEvery(interval, 0, self.gui.stopChan, func() error {
		self.gui.c.Refresh(types.RefreshOptions{Scope: []types.RefreshableView{types.FILES}})
		return nil
	})
}

// goEvery runs function on a periodic interval and returns a channel that can
// be used to trigger the callback immediately. If maxBackoff is > 0, the
// interval will be multiplied by up to maxBackoff on consecutive failures
// (tracked via consecutiveFailures). A manual retrigger resets the backoff.
// If maxBackoff is 0, the interval is fixed.
func (self *BackgroundRoutineMgr) goEvery(interval time.Duration, maxBackoff int, stop chan struct{}, function func() error) chan struct{} {
	done := make(chan struct{})
	retrigger := make(chan struct{})
	go utils.Safe(func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		doit := func(isManualTrigger bool) {
			if self.pauseBackgroundRefreshes {
				return
			}

			if isManualTrigger && maxBackoff > 0 {
				self.consecutiveFailures = 0
			}

			self.gui.c.OnWorker(func(gocui.Task) error {
				_ = function()
				done <- struct{}{}
				return nil
			})
			// waiting so that we don't bunch up refreshes if the refresh takes longer than the
			// interval, or if a retrigger comes in while we're still processing a timer-based one
			// (or vice versa)
			<-done

			if maxBackoff > 0 {
				multiplier := 1
				if self.consecutiveFailures > 0 {
					multiplier = 1 << self.consecutiveFailures
					if multiplier > maxBackoff {
						multiplier = maxBackoff
					}
				}
				ticker.Reset(interval * time.Duration(multiplier))
			}
		}
		for {
			select {
			case <-ticker.C:
				doit(false)
			case <-retrigger:
				ticker.Reset(interval)
				doit(true)
			case <-stop:
				return
			}
		}
	})
	return retrigger
}

func (self *BackgroundRoutineMgr) backgroundFetch() (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = self.gui.git.Sync.FetchBackground(ctx)

	self.gui.c.Refresh(types.RefreshOptions{Scope: []types.RefreshableView{types.BRANCHES, types.COMMITS, types.REMOTES, types.TAGS}, Mode: types.SYNC})

	if err == nil {
		err = self.gui.helpers.BranchesHelper.AutoForwardBranches()
	}

	return err
}

func (self *BackgroundRoutineMgr) triggerImmediateFetch() {
	if self.triggerFetch != nil {
		self.triggerFetch <- struct{}{}
	}
}
