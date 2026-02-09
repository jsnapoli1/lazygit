# Code Review: Background Fetch Reachability + Event-Driven File Refresh
**Date:** 2026-02-09

---

## File 1: `pkg/commands/oscommands/cmd_obj.go`

### 1.1 New field: `ctx context.Context`

```go
// if set, the command will be run with this context, allowing timeout/cancellation
ctx context.Context
```

**Justification:** The committed codebase has no way to cancel or time out a running command. Every `exec.Cmd` blocks until the process exits on its own. This field is the minimal addition needed to opt any command into deadline-based cancellation. It's a zero-value `nil` by default, so existing commands are completely unaffected. The field follows the same pattern as the existing `mutex` field -- optional, set via builder method, checked at execution time.

### 1.2 Builder and getter methods

```go
func (self *CmdObj) WithContext(ctx context.Context) *CmdObj {
	self.ctx = ctx
	return self
}

func (self *CmdObj) Context() context.Context {
	return self.ctx
}
```

**Justification:** Follows the existing builder pattern used throughout `CmdObj` (e.g., `DontLog()`, `WithMutex()`, `FailOnCredentialRequest()`). Returns `*CmdObj` for chaining. The getter returns `nil` when no context is set, which is the signal to `cmdWithContext` to use the original command unmodified. No need to modify `Clone()` because the struct copy (`*clone = *self`) already copies the `ctx` field.

---

## File 2: `pkg/commands/oscommands/cmd_obj_runner.go`

### 2.1 `cmdWithContext` helper function

```go
func cmdWithContext(cmdObj *CmdObj) *exec.Cmd {
	cmd := cmdObj.GetCmd()
	ctx := cmdObj.Context()
	if ctx == nil {
		return cmd
	}
	ctxCmd := exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)
	ctxCmd.Env = cmd.Env
	ctxCmd.Dir = cmd.Dir
	ctxCmd.Stdin = cmd.Stdin
	return ctxCmd
}
```

**Justification:** `exec.CommandContext` must be called at construction time -- you can't retroactively attach a context to an existing `exec.Cmd`. Since `CmdObj` constructs the `exec.Cmd` eagerly in `CmdObjBuilder.New()` (before the caller has a chance to set a context), we reconstruct the command here at execution time. The helper copies `Env`, `Dir`, and `Stdin` because those are the fields that `CmdObjBuilder` and other builder methods set. `Stdout` and `Stderr` are not copied because the runner methods set those themselves immediately after this call. When `ctx` is nil, the original command is returned unchanged -- zero overhead for all existing code paths.

### 2.2 Application in `RunWithOutputAux`

```go
cmd := cmdWithContext(cmdObj)
output, err := sanitisedCommandOutput(cmd.CombinedOutput())
```

**Justification:** Previously `cmdObj.GetCmd().CombinedOutput()`. Now routed through `cmdWithContext` so that if a context is set, the process will be killed when the deadline fires. `CombinedOutput` internally calls `Start` + `Wait`, and `exec.CommandContext` sets up a process kill on context cancellation before `Wait` returns.

### 2.3 Application in `RunWithOutputsAux`

```go
cmd := cmdWithContext(cmdObj)
cmd.Stdout = &outBuffer
cmd.Stderr = &errBuffer
err := cmd.Run()
```

**Justification:** Same pattern. This method sets `Stdout`/`Stderr` after getting the command, which is why `cmdWithContext` doesn't copy those fields.

### 2.4 Application in `runAndStreamAux`

```go
cmd := cmdWithContext(cmdObj)
```

**Justification:** This is the execution path used by background fetch (via `FailOnCredentialRequest` -> `runWithCredentialHandling` -> `runAndDetectCredentialRequest` -> `runAndStreamAux`). Without this change, the 30-second timeout on background fetch would have no effect because the actual command execution happens here. The context and the existing credential-failure mechanism (PTY close) are complementary -- the credential handler kills the process if it detects a password prompt, while the context kills it if the deadline expires.

---

## File 3: `pkg/commands/git_commands/remote_url_parser.go`

### 3.1 `defaultPorts` map

```go
var defaultPorts = map[string]int{
	"http":  80,
	"https": 443,
	"git":   9418,
	"ssh":   22,
}
```

**Justification:** go-git's `transport.NewEndpoint()` sets `Port` to 0 when no port is specified in the URL. We need to resolve that to the actual port for the TCP dial. These are the standard ports for the four protocols git supports over the network. This duplicates the `defaultPorts` map in go-git's `transport/common.go:153`, but that map is unexported so we can't reference it directly.

### 3.2 `ParseRemoteHostPort` function

```go
func ParseRemoteHostPort(remoteURL string) (string, bool) {
	endpoint, err := transport.NewEndpoint(remoteURL)
	if err != nil {
		return "", false
	}
```

**Justification:** Delegates URL parsing to go-git's `transport.NewEndpoint()` which already handles every URL format git supports: HTTPS, SSH, SCP-style (`git@github.com:user/repo`), `git://`, `file://`, and local paths. This is already vendored in the project. Returning `"", false` on parse error means the caller treats unparseable URLs as "not remote" and skips the reachability check (fail-open).

```go
	if endpoint.Protocol == "file" || endpoint.Host == "" {
		return "", false
	}
```

**Justification:** Local remotes (`file:///path` or bare paths) have no network host to dial. An empty host also indicates a local endpoint. Returning false tells the caller to skip the TCP check entirely.

```go
	port := endpoint.Port
	if port == 0 {
		defaultPort, ok := defaultPorts[endpoint.Protocol]
		if !ok {
			defaultPort = 22
		}
		port = defaultPort
	}
```

**Justification:** When the URL has no explicit port, `endpoint.Port` is 0. We resolve to the protocol's default. The fallback to port 22 handles SCP-style URLs (`git@host:repo`) where go-git sets the protocol to empty string. These are SSH connections, so 22 is correct.

---

## File 4: `pkg/commands/git_commands/remote_url_parser_test.go`

### 4.1 Test table

```go
func TestParseRemoteHostPort(t *testing.T) {
	tests := []struct { ... }{
		// HTTPS, SSH, SCP-style, git://, HTTP, SSH custom port, file://, local path, empty
	}
```

**Justification:** Covers every URL format that `transport.NewEndpoint()` handles. The SCP-style test (`git@github.com:user/repo.git` -> `github.com:22`) is the most important because it's the most common SSH URL format and has no explicit protocol. The custom port test (`ssh://git@github.com:2222/...` -> `github.com:2222`) verifies we respect explicit ports. The `file://`, local path, and empty string tests verify the fail-open behavior (all return `false`).

---

## File 5: `pkg/commands/git_commands/sync.go`

### 5.1 `FetchBackground` context parameter

```go
func (self *SyncCommands) FetchBackground(ctx context.Context) error {
	cmdObj := self.FetchBackgroundCmdObj()
	if ctx != nil {
		cmdObj.WithContext(ctx)
	}
	return cmdObj.Run()
}
```

**Justification:** The nil check allows callers that don't need a timeout to pass `nil` without penalty. `FetchBackgroundCmdObj()` is unchanged -- it still builds the command with `DontLog`, `FailOnCredentialRequest`, and `SuppressOutputUnlessError`. The context is layered on top. This keeps `FetchBackgroundCmdObj()` testable without needing a context (the existing test `TestSyncFetchBackground` calls `FetchBackgroundCmdObj()` directly and continues to work unchanged).

### 5.2 `CheckReachability` method

```go
func (self *SyncCommands) CheckReachability(remoteName string, timeout time.Duration) error {
	cmdArgs := NewGitCmd("ls-remote").
		Arg("--get-url", remoteName).
		ToArgv()

	url, err := self.cmd.New(cmdArgs).DontLog().RunWithOutput()
	if err != nil {
		return nil
	}
```

**Justification:** Uses `git ls-remote --get-url` to resolve the remote name to a URL. This handles git aliases and insteadOf rewrites. `.DontLog()` prevents this from appearing in the command log since it runs every fetch cycle. If this command fails (remote doesn't exist, git error), we return nil (fail-open) -- better to let the actual fetch attempt handle the error with a proper message than to silently skip it based on a URL resolution failure.

```go
	hostPort, isRemote := ParseRemoteHostPort(url)
	if !isRemote {
		return nil
	}

	conn, err := net.DialTimeout("tcp", hostPort, timeout)
	if err != nil {
		return fmt.Errorf("remote %s unreachable at %s: %w", remoteName, hostPort, err)
	}
	conn.Close()
	return nil
```

**Justification:** TCP dial to the actual service port (22 for SSH, 443 for HTTPS) rather than ICMP ping (requires root) or DNS lookup (can succeed even when host is unreachable due to caching). The 2-second timeout (passed by the caller) is short enough to not noticeably delay the fetch cycle, but long enough for a legitimate connection on a slow network. The error message includes the remote name and host:port for debuggability in logs. `conn.Close()` is called immediately -- we only need to know the port is open, not hold the connection.

---

## File 6: `pkg/gui/background.go`

### 6.1 New struct fields

```go
consecutiveFailures int
fileWatcher         *FileWatcher
```

**Justification:** `consecutiveFailures` tracks fetch failures for the backoff calculation. It's an int rather than a separate backoff struct because the logic is simple (shift and cap). `fileWatcher` is nil when the watcher failed to start or isn't configured, which is the signal to `startBackgroundFilesRefresh` to use the original polling interval.

### 6.2 `startBackgroundFetch` -- pre-check

```go
if err := self.gui.git.Sync.CheckReachability("origin", 2*time.Second); err != nil {
	self.gui.c.Log.Infof("Background fetch: remote unreachable, skipping: %v", err)
	self.consecutiveFailures++
	return nil
}
```

**Justification:** Runs before the fetch to avoid spawning a git process that will hang. Hardcoded to "origin" because that's the most common remote. When `git.fetchAll` is true, the fetch hits all remotes, but checking just origin is a reasonable proxy -- if origin is unreachable, the network is likely down. Returns `nil` (not an error) because unreachability is an expected condition, not a bug. The failure counter still increments to trigger backoff.

### 6.3 `startBackgroundFetch` -- error tracking

```go
if err != nil {
	self.gui.c.Log.Infof("Background fetch failed: %v", err)
	self.consecutiveFailures++
} else {
	self.consecutiveFailures = 0
}
```

**Justification:** The committed code silently discards all fetch errors (`_ = function()` in `goEvery`). Now errors are logged and tracked. Success resets the counter to 0, which resets the backoff interval. This means a single successful fetch after a period of failures immediately returns to the normal 60-second interval.

### 6.4 `startBackgroundFetch` -- backoff parameter

```go
self.triggerFetch = self.goEvery(userConfig.Refresher.FetchIntervalDuration(), 8, self.gui.stopChan, fetch)
```

**Justification:** Passes `maxBackoff=8` meaning the interval can grow up to 8x the base (60s * 8 = 480s = 8 minutes). This is aggressive enough to stop hammering an unreachable server but not so long that recovery takes forever.

### 6.5 `startFileWatcher`

```go
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
```

**Justification:** Called synchronously before `startBackgroundFilesRefresh` so that `self.fileWatcher` is set (or not) by the time the polling interval is calculated. Passes callbacks and paths rather than `*Gui` to keep `FileWatcher` decoupled. Uses `RepoPaths` which correctly resolves worktree paths -- `WorktreeGitDirPath()` points to `.git/worktrees/<name>/` for linked worktrees, not the shared `.git`.

```go
	if err != nil {
		self.gui.c.Log.Warnf("Failed to create file watcher, falling back to polling: %v", err)
		return
	}
	if err := watcher.Start(); err != nil {
		self.gui.c.Log.Warnf("Failed to start file watcher, falling back to polling: %v", err)
		watcher.Close()
		return
	}
```

**Justification:** Two-phase error handling (create then start) because `NewWatcher()` can fail on its own (e.g., syscall failure) and `Add()` calls in `Start()` can also fail (e.g., watch limit reached). On failure, `watcher.Close()` releases the fd, and `self.fileWatcher` stays nil which causes `startBackgroundFilesRefresh` to use the original polling interval.

### 6.6 `startBackgroundFilesRefresh` -- adaptive interval

```go
if self.fileWatcher != nil {
	const fallbackInterval = 5 * time.Minute
	if interval < fallbackInterval {
		interval = fallbackInterval
	}
}
```

**Justification:** When the watcher is active, polling is redundant for most events. The 5-minute fallback catches anything the watcher might miss (e.g., changes in a new subdirectory not yet watched). The `if interval < fallbackInterval` guard respects a user who has explicitly configured a longer interval.

### 6.7 `goEvery` -- unified with backoff

```go
func (self *BackgroundRoutineMgr) goEvery(interval time.Duration, maxBackoff int, stop chan struct{}, function func() error) chan struct{} {
```

**Justification:** A single scheduling primitive instead of two separate methods. When `maxBackoff` is 0, the backoff block is skipped entirely and the function behaves identically to the committed `goEvery`. This keeps callers like file refresh and debug logging simple (`maxBackoff=0`) while giving fetch the adaptive behavior it needs (`maxBackoff=8`).

```go
if isManualTrigger && maxBackoff > 0 {
	self.consecutiveFailures = 0
}
```

**Justification:** Manual retrigger (e.g., user switches repos) resets the backoff immediately. The `maxBackoff > 0` guard prevents this from running in the no-backoff case where `consecutiveFailures` is irrelevant.

```go
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
```

**Justification:** Exponential backoff via bit shift: 1 failure = 2x, 2 = 4x, 3 = 8x, 4+ = capped at `maxBackoff`. The cap prevents the interval from growing unboundedly. The ticker is reset after every execution (not just failures) so that a success resets the interval back to the base. When `consecutiveFailures` is 0, multiplier is 1, so `ticker.Reset(interval)` -- identical to the committed fixed-interval behavior.

### 6.8 `backgroundFetch` -- timeout

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
err = self.gui.git.Sync.FetchBackground(ctx)
```

**Justification:** 30 seconds is long enough for a normal fetch over a slow connection but short enough to prevent the indefinite hangs seen when the server is unreachable. The `defer cancel()` ensures the context resources are freed even if the fetch returns early. This is a defense-in-depth layer behind the 2-second TCP pre-check -- it catches cases where the pre-check passes (port is open) but the fetch itself stalls (e.g., server accepts connection but doesn't respond to the git protocol).

---

## File 7: `pkg/gui/filewatcher.go`

### 7.1 `FileWatcher` struct

```go
type FileWatcher struct {
	watcher            *fsnotify.Watcher
	log                *logrus.Entry
	refreshFn          func(types.RefreshOptions)
	pausedFn           func() bool
	worktreePath       string
	worktreeGitDirPath string
	repoGitDirPath     string
	debounceTimer      *time.Timer
	debounceMu         sync.Mutex
	debounceDelay      time.Duration
	pendingScopes      map[types.RefreshableView]bool
}
```

**Justification:** Takes callbacks and paths instead of a `*Gui` reference. This means the watcher can be tested without a running GUI and doesn't create coupling to GUI internals. `debounceDelay` is a field (not a constant) so it could be made configurable later. `pendingScopes` is a map-as-set to deduplicate scopes across multiple events in the debounce window.

### 7.2 `NewFileWatcher`

```go
func NewFileWatcher(
	log *logrus.Entry,
	refreshFn func(types.RefreshOptions),
	pausedFn func() bool,
	worktreePath string,
	worktreeGitDirPath string,
	repoGitDirPath string,
) (*FileWatcher, error) {
```

**Justification:** Constructor creates the `fsnotify.Watcher` eagerly so that the caller knows immediately if the OS doesn't support it or has run out of watch descriptors. `Start()` is separate so the caller can choose when to begin receiving events.

### 7.3 `Start` -- watch targets

```go
if err := self.watcher.Add(self.worktreePath); err != nil {
	return err
}
```

**Justification:** The working directory is the one required watch -- if we can't watch it, there's no point continuing. This is the only `Add` call whose error is returned.

```go
gitWatchPaths := []string{
	self.worktreeGitDirPath,
	filepath.Join(self.worktreeGitDirPath, "refs"),
	filepath.Join(self.worktreeGitDirPath, "refs", "heads"),
	filepath.Join(self.worktreeGitDirPath, "refs", "tags"),
	filepath.Join(self.worktreeGitDirPath, "refs", "remotes"),
}
```

**Justification:** `fsnotify` is not recursive -- it only watches the exact directory you specify. We watch the git dir root (for `HEAD`, `index`), plus each refs subdirectory individually. We watch `refs/` itself in addition to its children because some git operations create events at the `refs/` level.

```go
if self.repoGitDirPath != self.worktreeGitDirPath {
	gitWatchPaths = append(gitWatchPaths, ...)
}
```

**Justification:** In a linked worktree, `WorktreeGitDirPath()` points to `.git/worktrees/<name>/` which has the worktree-specific `HEAD` and `index`. But branches and tags are shared in the main repo's `.git/refs/`. Without watching both, branch/tag changes made from another worktree wouldn't trigger refreshes.

```go
for _, path := range gitWatchPaths {
	_ = self.watcher.Add(path)
}
```

**Justification:** Errors are deliberately ignored because some paths may not exist yet. For example, `refs/stash` doesn't exist until the first `git stash`, and `refs/remotes` might not exist in a repo with no remotes. Attempting and failing is cheaper than stat-checking each path, and the fallback polling will catch anything the watcher misses.

### 7.4 `eventLoop`

```go
func (self *FileWatcher) eventLoop() {
	for {
		select {
		case event, ok := <-self.watcher.Events:
			if !ok { return }
			self.handleEvent(event)
		case err, ok := <-self.watcher.Errors:
			if !ok { return }
			self.log.Errorf("filewatcher error: %v", err)
		}
	}
}
```

**Justification:** Standard fsnotify event loop pattern. The `!ok` checks handle the case where the watcher is closed (channels are closed). Errors are logged but don't stop the loop -- fsnotify errors are typically transient (e.g., too many events buffered).

### 7.5 `handleEvent` -- filtering and debouncing

```go
if self.pausedFn() {
	return
}
```

**Justification:** When the GUI is suspended (e.g., shelling out to an editor), filesystem events still arrive but we don't want to process them. The existing pause mechanism is reused via callback.

```go
if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
	return
}
```

**Justification:** Filters out `Chmod` events which are noise for our purposes. We only care about content changes (Write), new files (Create), deleted files (Remove), and moved files (Rename).

```go
if self.debounceTimer != nil {
	self.debounceTimer.Stop()
}
self.debounceTimer = time.AfterFunc(self.debounceDelay, self.flushPendingRefresh)
```

**Justification:** Sliding window debounce. Each event resets the 300ms timer. This coalesces rapid event bursts (like an editor doing write-to-temp + rename, or `git checkout` touching HEAD + index + multiple files) into a single refresh. The scopes accumulate in `pendingScopes` during the window, so a HEAD change followed by an index change within 300ms becomes one refresh covering both BRANCHES+COMMITS and FILES.

### 7.6 `classifyEvent` -- path routing

```go
if relPath, ok := relativeUnder(path, self.worktreeGitDirPath); ok {
	return self.classifyGitEvent(relPath)
}
```

**Justification:** Two-step classification: first check if the event is under a git directory (worktree-specific or shared), then check if it's a working tree file. Git directory events are routed to `classifyGitEvent` for fine-grained scope mapping. The order matters -- `.git/` is inside the working directory, so we check for it first to avoid classifying `.git/index` as a FILES change.

```go
base := filepath.Base(path)
if base == ".git" {
	return nil
}
return []types.RefreshableView{types.FILES}
```

**Justification:** The `.git` directory/file itself changing (e.g., in a worktree where `.git` is a file) is not a working tree change. Everything else in the working directory is a file change.

### 7.7 `classifyGitEvent` -- scope mapping

```go
case relPath == "HEAD":
	return []types.RefreshableView{types.BRANCHES, types.COMMITS}
case relPath == "index":
	return []types.RefreshableView{types.FILES}
case relPath == "index.lock":
	return nil
```

**Justification:** `HEAD` changing means a branch switch or commit -- both BRANCHES and COMMITS views need updating. `index` changing means files were staged/unstaged. `index.lock` is a transient lock file created during staging operations -- refreshing on it would cause spurious double-refreshes since the actual `index` write follows immediately.

```go
case strings.HasPrefix(relPath, filepath.Join("refs", "heads")+string(filepath.Separator)):
	return []types.RefreshableView{types.BRANCHES}
case relPath == filepath.Join("refs", "heads"):
	return []types.RefreshableView{types.BRANCHES}
```

**Justification:** Branch refs are files under `refs/heads/`. The second case handles events on the `refs/heads` directory itself (e.g., when the directory is created). Same pattern repeats for tags, remotes, and stash.

```go
case relPath == "MERGE_HEAD" || strings.HasPrefix(relPath, "rebase-merge"+string(filepath.Separator)):
	return []types.RefreshableView{types.COMMITS, types.FILES}
```

**Justification:** `MERGE_HEAD` appearing/disappearing indicates merge start/end. `rebase-merge/` contents changing indicates rebase progress. Both affect the commit list and may affect file status.

```go
default:
	return nil
```

**Justification:** Other `.git/` changes (logs, hooks, config, COMMIT_EDITMSG, etc.) don't map to any view and are ignored. This prevents unnecessary refreshes from git's internal bookkeeping.

### 7.8 `flushPendingRefresh`

```go
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
```

**Justification:** Called by `time.AfterFunc`, which runs in its own goroutine -- hence the mutex. The map is replaced (not cleared) to avoid holding the lock during the refresh call. `ASYNC` mode means the refresh returns immediately and runs on a worker thread, matching the committed behavior of background refreshes.

### 7.9 `Close`

```go
func (self *FileWatcher) Close() error {
	self.debounceMu.Lock()
	if self.debounceTimer != nil {
		self.debounceTimer.Stop()
	}
	self.debounceMu.Unlock()
	return self.watcher.Close()
}
```

**Justification:** Stops the debounce timer to prevent a pending flush from firing after the watcher is closed. `watcher.Close()` closes the event and error channels, which causes `eventLoop` to return via the `!ok` checks. The mutex prevents a race between `Close` and a concurrent `handleEvent`.

### 7.10 `relativeUnder` helper

```go
func relativeUnder(target, base string) (string, bool) {
	if !strings.HasSuffix(base, string(filepath.Separator)) {
		base += string(filepath.Separator)
	}
	if strings.HasPrefix(target, base) {
		return target[len(base):], true
	}
	return "", false
}
```

**Justification:** `filepath.Rel` is heavier than needed and can return `../` paths. We only need to know if a path is strictly under a base directory and extract the relative portion. Adding the trailing separator prevents false matches (e.g., `/foo/bar` should not match base `/foo/b`).

---

## File 8: `pkg/gui/gui.go`

### 8.1 File watcher cleanup

```go
if gui.BackgroundRoutineMgr.fileWatcher != nil {
	gui.BackgroundRoutineMgr.fileWatcher.Close()
}
```

**Justification:** Placed after `close(gui.stopChan)` which stops the background goroutines, and before the quit/restart logic. The nil check handles the case where the watcher failed to start. Closing releases OS file descriptors (inotify fd on Linux, kqueue fd on macOS). Without this, restarting the GUI within the same process (e.g., repo switch) would leak watchers.
