package gui

import (
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/jesseduffield/lazygit/pkg/gui/types"
	"github.com/jesseduffield/lazygit/pkg/utils"
	"github.com/sirupsen/logrus"
)

// FileWatcher uses OS-level filesystem events to trigger targeted refreshes
// instead of polling. It debounces rapid events and classifies changes by
// path to determine which views need refreshing.
type FileWatcher struct {
	watcher *fsnotify.Watcher
	log     *logrus.Entry

	// refreshFn triggers a refresh of the specified scopes
	refreshFn func(types.RefreshOptions)

	// pausedFn returns true if background refreshes are paused
	pausedFn func() bool

	// paths used for event classification
	worktreePath       string
	worktreeGitDirPath string
	repoGitDirPath     string

	// debounce state
	debounceTimer *time.Timer
	debounceMu    sync.Mutex
	debounceDelay time.Duration
	pendingScopes map[types.RefreshableView]bool
}

func NewFileWatcher(
	log *logrus.Entry,
	refreshFn func(types.RefreshOptions),
	pausedFn func() bool,
	worktreePath string,
	worktreeGitDirPath string,
	repoGitDirPath string,
) (*FileWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	return &FileWatcher{
		watcher:            watcher,
		log:                log,
		refreshFn:          refreshFn,
		pausedFn:           pausedFn,
		worktreePath:       worktreePath,
		worktreeGitDirPath: worktreeGitDirPath,
		repoGitDirPath:     repoGitDirPath,
		debounceDelay:      300 * time.Millisecond,
		pendingScopes:      make(map[types.RefreshableView]bool),
	}, nil
}

// Start begins watching the working directory and relevant .git subdirectories.
func (self *FileWatcher) Start() error {
	// Watch the working directory for file changes
	if err := self.watcher.Add(self.worktreePath); err != nil {
		return err
	}

	// Watch .git directory and key subdirectories.
	// Errors are ignored for paths that don't exist yet (e.g. refs/stash
	// won't exist until the first stash is created).
	gitWatchPaths := []string{
		self.worktreeGitDirPath,
		filepath.Join(self.worktreeGitDirPath, "refs"),
		filepath.Join(self.worktreeGitDirPath, "refs", "heads"),
		filepath.Join(self.worktreeGitDirPath, "refs", "tags"),
		filepath.Join(self.worktreeGitDirPath, "refs", "remotes"),
	}

	// If we're in a linked worktree, also watch the shared repo's refs
	if self.repoGitDirPath != self.worktreeGitDirPath {
		gitWatchPaths = append(gitWatchPaths,
			self.repoGitDirPath,
			filepath.Join(self.repoGitDirPath, "refs"),
			filepath.Join(self.repoGitDirPath, "refs", "heads"),
			filepath.Join(self.repoGitDirPath, "refs", "tags"),
			filepath.Join(self.repoGitDirPath, "refs", "remotes"),
		)
	}

	for _, path := range gitWatchPaths {
		_ = self.watcher.Add(path)
	}

	go utils.Safe(self.eventLoop)
	return nil
}

func (self *FileWatcher) eventLoop() {
	for {
		select {
		case event, ok := <-self.watcher.Events:
			if !ok {
				return
			}
			self.handleEvent(event)

		case err, ok := <-self.watcher.Errors:
			if !ok {
				return
			}
			self.log.Errorf("filewatcher error: %v", err)
		}
	}
}

func (self *FileWatcher) handleEvent(event fsnotify.Event) {
	if self.pausedFn() {
		return
	}

	// Only care about writes, creates, removes, and renames
	if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
		return
	}

	scopes := self.classifyEvent(event.Name)
	if len(scopes) == 0 {
		return
	}

	self.debounceMu.Lock()
	defer self.debounceMu.Unlock()

	for _, scope := range scopes {
		self.pendingScopes[scope] = true
	}

	if self.debounceTimer != nil {
		self.debounceTimer.Stop()
	}
	self.debounceTimer = time.AfterFunc(self.debounceDelay, self.flushPendingRefresh)
}

func (self *FileWatcher) classifyEvent(path string) []types.RefreshableView {
	// Check if the event is under a git directory
	if relPath, ok := relativeUnder(path, self.worktreeGitDirPath); ok {
		return self.classifyGitEvent(relPath)
	}
	if self.repoGitDirPath != self.worktreeGitDirPath {
		if relPath, ok := relativeUnder(path, self.repoGitDirPath); ok {
			return self.classifyGitEvent(relPath)
		}
	}

	// Working tree file change
	base := filepath.Base(path)
	// Ignore the .git directory/file itself
	if base == ".git" {
		return nil
	}

	return []types.RefreshableView{types.FILES}
}

func (self *FileWatcher) classifyGitEvent(relPath string) []types.RefreshableView {
	switch {
	case relPath == "HEAD":
		return []types.RefreshableView{types.BRANCHES, types.COMMITS}
	case relPath == "index":
		return []types.RefreshableView{types.FILES}
	case relPath == "index.lock":
		// Lock files are transient; ignore them
		return nil
	case strings.HasPrefix(relPath, filepath.Join("refs", "heads")+string(filepath.Separator)):
		return []types.RefreshableView{types.BRANCHES}
	case relPath == filepath.Join("refs", "heads"):
		return []types.RefreshableView{types.BRANCHES}
	case strings.HasPrefix(relPath, filepath.Join("refs", "tags")+string(filepath.Separator)):
		return []types.RefreshableView{types.TAGS}
	case relPath == filepath.Join("refs", "tags"):
		return []types.RefreshableView{types.TAGS}
	case strings.HasPrefix(relPath, filepath.Join("refs", "remotes")+string(filepath.Separator)):
		return []types.RefreshableView{types.REMOTES}
	case relPath == filepath.Join("refs", "remotes"):
		return []types.RefreshableView{types.REMOTES}
	case relPath == filepath.Join("refs", "stash") || strings.HasPrefix(relPath, filepath.Join("refs", "stash")+string(filepath.Separator)):
		return []types.RefreshableView{types.STASH}
	case relPath == "MERGE_HEAD" || strings.HasPrefix(relPath, "rebase-merge"+string(filepath.Separator)):
		return []types.RefreshableView{types.COMMITS, types.FILES}
	default:
		return nil
	}
}

func (self *FileWatcher) flushPendingRefresh() {
	self.debounceMu.Lock()
	scopes := make([]types.RefreshableView, 0, len(self.pendingScopes))
	for scope := range self.pendingScopes {
		scopes = append(scopes, scope)
	}
	self.pendingScopes = make(map[types.RefreshableView]bool)
	self.debounceMu.Unlock()

	if len(scopes) > 0 {
		self.refreshFn(types.RefreshOptions{
			Scope: scopes,
			Mode:  types.ASYNC,
		})
	}
}

// Close shuts down the file watcher and releases resources.
func (self *FileWatcher) Close() error {
	self.debounceMu.Lock()
	if self.debounceTimer != nil {
		self.debounceTimer.Stop()
	}
	self.debounceMu.Unlock()
	return self.watcher.Close()
}

// relativeUnder returns the relative path of target under base, and true,
// if target is under base. Otherwise returns "", false.
func relativeUnder(target, base string) (string, bool) {
	// Ensure base ends with separator for prefix matching
	if !strings.HasSuffix(base, string(filepath.Separator)) {
		base += string(filepath.Separator)
	}
	if strings.HasPrefix(target, base) {
		return target[len(base):], true
	}
	return "", false
}
