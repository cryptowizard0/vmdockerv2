# Runtime Startup Responsibility Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Collapse the container startup path so the Go adapter is the ENTRYPOINT (PID 1) that supervises an author-owned `start.sh`, the host enforces confinement it can guarantee, and readiness is host-authoritative.

**Architecture:** Two repos, two phases. **Phase 1 (vmdocker_agent):** the adapter absorbs the deleted shell wrapper — it runs runtime-type-specific prep, exports workspace env, spawns/supervises `start.sh`, gates `/vmm/health` on runtime-type-specific readiness, and performs PID 1 init (reap + `SIGTERM` forward). Default `start.sh` templates ship per runtime. **Phase 2 (vmdockerv2):** the host enforces a non-root `User` and `MemorySwap=Memory`, runs a one-shot Docker-daemon confinement self-check at node startup, and the modulebuild Dockerfile generator drops the wrapper while keeping `ENV RUNTIME_TYPE`.

**Tech Stack:** Go 1.x, gin (adapter HTTP), Docker SDK `github.com/docker/docker` v28.0.4, POSIX `sh` (start.sh templates), `go test`.

## Global Constraints

- Module format is `hymx.vmdockerv2.v0.0.1` — do not change it.
- **`RUNTIME_TYPE` env is retained.** `runtime.newRuntime` selects the backend from it; the generated Dockerfile keeps `ENV RUNTIME_TYPE`. Only the wrapper's `bootstrap/*.sh` dispatch is removed.
- Enforced non-root uid must be the `hymx` user the Dockerfile creates.
- `/vmm/health` readiness contract: HTTP **200 = ready**, any other status = not ready (host `waitForContainerReady` only accepts `http.StatusOK`).
- Runtime-type readiness: **openclaw** = gateway ping OK; **claude** = `claude` CLI on `PATH`; **test / telegramcustomer / default** = always ready.
- Go module paths: adapter = `github.com/cryptowizard0/vmdocker_agent`; host = `github.com/cryptowizard0/vmdockerv2`.
- `start.sh` templates are POSIX `sh` (`#!/bin/sh`, `set -eu`).
- Repos: vmdocker_agent at `/Users/webbergao/work/src/HymxWorkspace/vmdocker_agent`; vmdockerv2 at `/Users/webbergao/work/src/HymxWorkspace/vmdockerv2`.
- Dev machine is macOS (darwin); the adapter runs in Linux containers. Tests must pass on darwin — do not write tests that depend on real PID 1 / subreaper semantics; test the wiring via injectable seams instead.

## File Structure

**vmdocker_agent (Phase 1)**
- Create `runtime/launcher.go` — `Launcher` interface + `LauncherFor` + `CurrentRuntimeType` + three implementations. Boot-time prep and readiness, decoupled from `IRuntime` (which is the lazily-created spawn instance).
- Create `runtime/launcher_test.go`.
- Create `supervisor/supervisor.go` — spawn `start.sh` in its own process group, capture logs, reap zombies (injectable), forward signals to the group.
- Create `supervisor/supervisor_test.go`.
- Modify `server/server.go` — `Server` gains `launcher` + `sup`; `Run` orchestrates prepare → spawn → reap → serve → forward.
- Modify `server/api.go` — `health` gates on `launcher.Ready`.
- Modify `server/api_test.go` — health readiness tests.
- Create `startup/openclaw.sh`, `startup/claude.sh` — default `start.sh` templates.
- Delete `start-vmdocker-agent.sh`, `bootstrap/openclaw.sh`, `bootstrap/claude.sh`, `scripts/test_startup_hook_isolation.sh`.
- Modify `Dockerfile.openclaw`, `Dockerfile.claude`, `Dockerfile.telegramcustomer`.

**vmdockerv2 (Phase 2)**
- Modify `vmdocker/runtimemanager/docker.go` — non-root `User`; `MemorySwap = Memory`; call node self-check from `NewDockerManager`.
- Modify `vmdocker/runtimemanager/docker_test.go`.
- Create `vmdocker/runtimemanager/nodecheck.go` — `RunNodeConfinementCheck`.
- Create `vmdocker/runtimemanager/nodecheck_test.go`.
- Modify `vmdocker/modulebuild/dockerfile.go` + `dockerfile_test.go` — drop `WrapperSrc`, ENTRYPOINT = adapter, keep `ENV RUNTIME_TYPE`.
- Modify `vmdocker/modulebuild/build.go` + `build_test.go` — drop `WrapperPath` and wrapper staging.

---

# Phase 1 — vmdocker_agent adapter

### Task 1: Runtime-type Launcher (prep + readiness)

Introduces a per-`RUNTIME_TYPE` boot abstraction: `Prepare()` runs setup and returns env to export before `start.sh`; `Ready()` is the readiness probe the health endpoint gates on. Decoupled from `IRuntime` because the engine boots at container start, while the runtime instance is created lazily on `/vmm/spawn`.

**Files:**
- Create: `runtime/launcher.go`
- Test: `runtime/launcher_test.go`

**Interfaces:**
- Consumes: `utils.PrepareOpenclawRuntime(EnvLookup, UserHomeDirFunc) (OpenclawPaths, error)`; `openclaw.LoadConfigFromEnv() schema.Config`; `openclaw.NewHTTPGatewayClient(schema.Config) *HTTPGatewayClient` with `Init(ctx) error`.
- Produces:
  - `type Launcher interface { Prepare() ([]string, error); Ready(ctx context.Context) error }`
  - `func CurrentRuntimeType() string` — returns `RUNTIME_TYPE` env or `"test"`.
  - `func LauncherFor(runtimeType string) Launcher`

- [ ] **Step 1: Write the failing test**

```go
// runtime/launcher_test.go
package runtime

import (
	"context"
	"os"
	"testing"
)

func TestCurrentRuntimeTypeDefaultsToTest(t *testing.T) {
	t.Setenv("RUNTIME_TYPE", "")
	if got := CurrentRuntimeType(); got != RuntimeTypeTest {
		t.Fatalf("want %q, got %q", RuntimeTypeTest, got)
	}
}

func TestLauncherForClaudeReadyRequiresCLI(t *testing.T) {
	l := LauncherFor(RuntimeTypeClaude)

	// Empty PATH -> claude not found -> not ready.
	t.Setenv("PATH", "")
	if err := l.Ready(context.Background()); err == nil {
		t.Fatal("want not-ready when claude is absent, got nil")
	}

	// A dir containing an executable named "claude" -> ready.
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/claude", []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	if err := l.Ready(context.Background()); err != nil {
		t.Fatalf("want ready when claude present, got %v", err)
	}
}

func TestLauncherForTestAlwaysReady(t *testing.T) {
	l := LauncherFor(RuntimeTypeTest)
	env, err := l.Prepare()
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if len(env) != 0 {
		t.Fatalf("test launcher should export no env, got %v", env)
	}
	if err := l.Ready(context.Background()); err != nil {
		t.Fatalf("test launcher should always be ready, got %v", err)
	}
}

func TestLauncherForUnknownUsesAlwaysReady(t *testing.T) {
	if err := LauncherFor("telegramcustomer").Ready(context.Background()); err != nil {
		t.Fatalf("unknown/default launcher should be ready, got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/webbergao/work/src/HymxWorkspace/vmdocker_agent && go test ./runtime/ -run TestLauncher -v`
Expected: FAIL — `undefined: LauncherFor` / `CurrentRuntimeType`.

- [ ] **Step 3: Write minimal implementation**

```go
// runtime/launcher.go
package runtime

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/cryptowizard0/vmdocker_agent/runtime/openclaw"
	"github.com/cryptowizard0/vmdocker_agent/utils"
)

// Launcher is the runtime-type-specific container-boot behavior: preparation
// done before start.sh runs, and the readiness probe /vmm/health gates on. It
// is independent of IRuntime (the spawn instance created lazily on /vmm/spawn).
type Launcher interface {
	// Prepare runs pre-start.sh setup and returns "KEY=value" assignments to
	// export before spawning start.sh.
	Prepare() ([]string, error)
	// Ready returns nil when the runtime engine is ready to serve.
	Ready(ctx context.Context) error
}

// CurrentRuntimeType returns the configured RUNTIME_TYPE, defaulting to test.
func CurrentRuntimeType() string {
	if t := os.Getenv("RUNTIME_TYPE"); t != "" {
		return t
	}
	return RuntimeTypeTest
}

// LauncherFor maps a runtime type to its Launcher. Unknown types (including
// telegramcustomer) use the always-ready launcher: no engine prep, no gating.
func LauncherFor(runtimeType string) Launcher {
	switch runtimeType {
	case RuntimeTypeOpenclaw:
		return openclawLauncher{}
	case RuntimeTypeClaude:
		return claudeLauncher{}
	default:
		return alwaysReadyLauncher{}
	}
}

type alwaysReadyLauncher struct{}

func (alwaysReadyLauncher) Prepare() ([]string, error)        { return nil, nil }
func (alwaysReadyLauncher) Ready(context.Context) error       { return nil }

type claudeLauncher struct{}

func (claudeLauncher) Prepare() ([]string, error) { return nil, nil }

func (claudeLauncher) Ready(context.Context) error {
	if _, err := exec.LookPath("claude"); err != nil {
		return fmt.Errorf("claude CLI not on PATH: %w", err)
	}
	return nil
}

type openclawLauncher struct{}

func (openclawLauncher) Prepare() ([]string, error) {
	paths, err := utils.PrepareOpenclawRuntime(os.Getenv, os.UserHomeDir)
	if err != nil {
		return nil, fmt.Errorf("prepare openclaw runtime: %w", err)
	}
	return []string{
		"OPENCLAW_STATE_DIR=" + paths.StateDir,
		"OPENCLAW_CONFIG_PATH=" + paths.ConfigPath,
		"OPENCLAW_GATEWAY_LOG_PATH=" + paths.GatewayLogPath,
	}, nil
}

func (openclawLauncher) Ready(ctx context.Context) error {
	client := openclaw.NewHTTPGatewayClient(openclaw.LoadConfigFromEnv())
	if err := client.Init(ctx); err != nil {
		return fmt.Errorf("openclaw gateway not ready: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/webbergao/work/src/HymxWorkspace/vmdocker_agent && go test ./runtime/ -run TestLauncher -v && go build ./...`
Expected: PASS; build clean.

- [ ] **Step 5: Commit**

```bash
cd /Users/webbergao/work/src/HymxWorkspace/vmdocker_agent
git add runtime/launcher.go runtime/launcher_test.go
git commit -m "feat(adapter): add runtime-type launcher for prep + readiness"
```

---

### Task 2: start.sh supervisor (spawn + log capture)

Spawns `start.sh` in its own process group with output captured to a log file. Process-group isolation lets the adapter later signal the engine even after `start.sh` itself exits.

**Files:**
- Create: `supervisor/supervisor.go`
- Test: `supervisor/supervisor_test.go`

**Interfaces:**
- Produces:
  - `type Supervisor struct { ... }`
  - `func New(hookPath, logPath string) *Supervisor`
  - `func (s *Supervisor) Start() error` — starts `sh <hookPath>` (Setpgid), stdout+stderr → `logPath`. No-op (returns nil) if `hookPath` does not exist.
  - `func (s *Supervisor) PGID() int`

- [ ] **Step 1: Write the failing test**

```go
// supervisor/supervisor_test.go
package supervisor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStartCapturesOutput(t *testing.T) {
	dir := t.TempDir()
	hook := filepath.Join(dir, "start.sh")
	logPath := filepath.Join(dir, "start.log")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho hello-stdout\necho hello-stderr 1>&2\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	s := New(hook, logPath)
	if err := s.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	var data []byte
	for time.Now().Before(deadline) {
		data, _ = os.ReadFile(logPath)
		if strings.Contains(string(data), "hello-stdout") && strings.Contains(string(data), "hello-stderr") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("log did not capture output, got %q", string(data))
}

func TestStartMissingHookIsNoOp(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "nope.sh"), filepath.Join(t.TempDir(), "x.log"))
	if err := s.Start(); err != nil {
		t.Fatalf("missing hook should be a no-op, got %v", err)
	}
	if s.PGID() != 0 {
		t.Fatalf("no process should have been started, PGID=%d", s.PGID())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/webbergao/work/src/HymxWorkspace/vmdocker_agent && go test ./supervisor/ -v`
Expected: FAIL — package/symbols undefined.

- [ ] **Step 3: Write minimal implementation**

```go
// supervisor/supervisor.go
package supervisor

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
)

// Supervisor launches the untrusted start.sh in its own process group and
// captures its output. The adapter (PID 1) uses PGID to forward signals to the
// engine even after start.sh exits.
type Supervisor struct {
	hookPath string
	logPath  string

	mu   sync.Mutex
	cmd  *exec.Cmd
	pgid int
	// reap is the zombie-reaping function; injectable for tests. Defaults to
	// reapZombies. See Task 3.
	reap func()
}

func New(hookPath, logPath string) *Supervisor {
	return &Supervisor{hookPath: hookPath, logPath: logPath, reap: reapZombies}
}

// Start launches `sh <hookPath>` with stdout+stderr redirected to logPath in a
// new process group. A missing hook is a no-op.
func (s *Supervisor) Start() error {
	if _, err := os.Stat(s.hookPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat start hook: %w", err)
	}

	logFile, err := os.OpenFile(s.logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open start log: %w", err)
	}

	cmd := exec.Command("sh", s.hookPath)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("start start.sh: %w", err)
	}

	s.mu.Lock()
	s.cmd = cmd
	s.pgid = cmd.Process.Pid // equals the new pgid because Setpgid is set
	s.mu.Unlock()
	return nil
}

func (s *Supervisor) PGID() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pgid
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/webbergao/work/src/HymxWorkspace/vmdocker_agent && go test ./supervisor/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/webbergao/work/src/HymxWorkspace/vmdocker_agent
git add supervisor/supervisor.go supervisor/supervisor_test.go
git commit -m "feat(adapter): supervisor spawns start.sh with log capture"
```

---

### Task 3: Supervisor PID 1 duties (reap + signal forward)

Adds the two PID 1 init duties: reaping zombies and forwarding a termination signal to the engine's process group. The reap loop's wiring is tested via an injectable `reap` seam (real zombie reaping is not portable to the darwin dev machine).

**Files:**
- Modify: `supervisor/supervisor.go`
- Test: `supervisor/supervisor_test.go`

**Interfaces:**
- Produces:
  - `func reapZombies()` — drains exited children via `syscall.Wait4(-1, ..., WNOHANG)`.
  - `func (s *Supervisor) ReapLoop(sigchld <-chan os.Signal)` — calls `s.reap` on each signal.
  - `func (s *Supervisor) Forward(sig syscall.Signal) error` — sends `sig` to the start.sh process group.

- [ ] **Step 1: Write the failing test**

```go
// append to supervisor/supervisor_test.go
import (
	// add to the existing import block:
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
)

func TestReapLoopInvokesReapOnSignal(t *testing.T) {
	s := New("", "") // no process needed for this wiring test
	var calls int32
	s.reap = func() { atomic.AddInt32(&calls, 1) }

	ch := make(chan os.Signal, 1)
	go s.ReapLoop(ch)

	ch <- syscall.SIGCHLD
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&calls) >= 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("reap was not invoked on SIGCHLD, calls=%d", atomic.LoadInt32(&calls))
}

func TestForwardSignalsProcessGroup(t *testing.T) {
	dir := t.TempDir()
	hook := filepath.Join(dir, "start.sh")
	marker := filepath.Join(dir, "term.marker")
	// Trap TERM: create the marker and exit; otherwise wait.
	script := "#!/bin/sh\ntrap 'touch " + marker + "; exit 0' TERM\n(while true; do sleep 0.1; done)\n"
	if err := os.WriteFile(hook, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	s := New(hook, filepath.Join(dir, "start.log"))
	// Reap so the child does not linger as a zombie after it exits.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGCHLD)
	defer signal.Stop(sig)
	go s.ReapLoop(sig)

	if err := s.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	time.Sleep(200 * time.Millisecond) // let the trap install

	if err := s.Forward(syscall.SIGTERM); err != nil {
		t.Fatalf("forward: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("TERM was not delivered to the process group")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/webbergao/work/src/HymxWorkspace/vmdocker_agent && go test ./supervisor/ -run 'TestReapLoop|TestForward' -v`
Expected: FAIL — `ReapLoop` / `Forward` / `reapZombies` undefined.

- [ ] **Step 3: Write minimal implementation**

```go
// append to supervisor/supervisor.go
import (
	// add to the existing import block:
	"os"
)

// reapZombies drains all reapable children. As PID 1 the adapter inherits
// orphaned engine processes; this prevents zombie accumulation.
func reapZombies() {
	for {
		var ws syscall.WaitStatus
		pid, err := syscall.Wait4(-1, &ws, syscall.WNOHANG, nil)
		if pid <= 0 || err != nil {
			return
		}
	}
}

// ReapLoop reaps children whenever a SIGCHLD arrives on sigchld.
func (s *Supervisor) ReapLoop(sigchld <-chan os.Signal) {
	for range sigchld {
		s.reap()
	}
}

// Forward sends sig to the start.sh process group (negative pgid). No-op if no
// process was started.
func (s *Supervisor) Forward(sig syscall.Signal) error {
	pgid := s.PGID()
	if pgid == 0 {
		return nil
	}
	if err := syscall.Kill(-pgid, sig); err != nil {
		return fmt.Errorf("signal process group %d: %w", pgid, err)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/webbergao/work/src/HymxWorkspace/vmdocker_agent && go test ./supervisor/ -v`
Expected: PASS (all supervisor tests).

- [ ] **Step 5: Commit**

```bash
cd /Users/webbergao/work/src/HymxWorkspace/vmdocker_agent
git add supervisor/supervisor.go supervisor/supervisor_test.go
git commit -m "feat(adapter): supervisor reaps zombies and forwards signals"
```

---

### Task 4: Health endpoint gates on readiness

Replaces the unconditional `200` with a runtime-type-specific readiness gate. Ready → `200`; not ready → `503`.

**Files:**
- Modify: `server/server.go` (add `launcher` field + constructor wiring)
- Modify: `server/api.go` (`health`)
- Test: `server/api_test.go`

**Interfaces:**
- Consumes: `runtime.Launcher`, `runtime.LauncherFor`, `runtime.CurrentRuntimeType` (Task 1).
- Produces: `Server.launcher runtime.Launcher`; `health` returns 200 when `launcher.Ready(ctx) == nil`, else 503.

- [ ] **Step 1: Write the failing test**

```go
// append to server/api_test.go
type stubLauncher struct{ err error }

func (s stubLauncher) Prepare() ([]string, error)  { return nil, nil }
func (s stubLauncher) Ready(context.Context) error { return s.err }

func TestHealthReadyReturns200(t *testing.T) {
	s := setupTestServer(t)
	s.launcher = stubLauncher{err: nil}

	w := performJSONRequest(t, s, http.MethodPost, "/vmm/health", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
}

func TestHealthNotReadyReturns503(t *testing.T) {
	s := setupTestServer(t)
	s.launcher = stubLauncher{err: errors.New("engine down")}

	w := performJSONRequest(t, s, http.MethodPost, "/vmm/health", nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", w.Code)
	}
}
```

Note: use the existing `setupTestServer(t)` helper (it registers routes on `s.engine`, which `performJSONRequest` calls) — do NOT register routes on a fresh `gin.New()`, that would 404. The `health` handler reads `s.launcher` at call time, so overriding it after `setupTestServer` works. Add imports `"context"` and `"errors"` to `server/api_test.go` (neither is currently imported). The existing `TestHealth` must still pass: `setupTestServer` calls `New(0)`, and with `RUNTIME_TYPE` unset the launcher is the always-ready one → 200.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/webbergao/work/src/HymxWorkspace/vmdocker_agent && go test ./server/ -run TestHealth -v`
Expected: FAIL — `s.launcher` undefined; health returns 200 even for the not-ready case.

- [ ] **Step 3: Write minimal implementation**

In `server/server.go`, add the field and wire it in `New`:

```go
// server/server.go — add import
"github.com/cryptowizard0/vmdocker_agent/runtime"

// in the Server struct, add:
	launcher runtime.Launcher

// in New(), set it:
func New(port int) *Server {
	engine := gin.Default()
	return &Server{
		engine:   engine,
		port:     port,
		aoPath:   getEnvOrDefault("AO_PATH", "./ao/2.0.1"),
		launcher: runtime.LauncherFor(runtime.CurrentRuntimeType()),
	}
}
```

In `server/api.go`, replace `health`:

```go
func (s *Server) health(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	if err := s.launcher.Ready(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "not_ready",
			"reason": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
```

(`context` and `time` are already imported in `server/api.go`.)

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/webbergao/work/src/HymxWorkspace/vmdocker_agent && go test ./server/ -run TestHealth -v && go build ./...`
Expected: PASS; build clean.

- [ ] **Step 5: Commit**

```bash
cd /Users/webbergao/work/src/HymxWorkspace/vmdocker_agent
git add server/server.go server/api.go server/api_test.go
git commit -m "feat(adapter): gate /vmm/health on runtime-type readiness"
```

---

### Task 5: Wire boot orchestration into Server.Run

The adapter becomes the ENTRYPOINT: on `Run`, it prepares env, spawns `start.sh`, starts the reap loop, serves the API, and on `SIGTERM` forwards to the engine before shutting down.

**Files:**
- Modify: `server/server.go`
- Test: `server/api_test.go` (orchestration seam test)

**Interfaces:**
- Consumes: `supervisor.New`, `Supervisor.Start`, `Supervisor.ReapLoop`, `Supervisor.Forward` (Tasks 2–3); `Launcher.Prepare` (Task 1).
- Produces: `func (s *Server) bootRuntime() error` — Prepare→export env→spawn start.sh→start reap loop. Called by `Run` before serving.

- [ ] **Step 1: Write the failing test**

```go
// append to server/api_test.go
func TestBootRuntimeExportsEnvFromLauncher(t *testing.T) {
	s := New(0)
	s.launcher = envLauncher{env: []string{"BOOT_TEST_KEY=boot-test-val"}}
	s.startHookPath = filepath.Join(t.TempDir(), "absent.sh") // missing -> spawn no-op

	if err := s.bootRuntime(); err != nil {
		t.Fatalf("bootRuntime: %v", err)
	}
	if got := os.Getenv("BOOT_TEST_KEY"); got != "boot-test-val" {
		t.Fatalf("env not exported, got %q", got)
	}
}

type envLauncher struct{ env []string }

func (e envLauncher) Prepare() ([]string, error)   { return e.env, nil }
func (e envLauncher) Ready(context.Context) error  { return nil }
```

Add imports `"os"` and `"path/filepath"` to `server/api_test.go` if absent.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/webbergao/work/src/HymxWorkspace/vmdocker_agent && go test ./server/ -run TestBootRuntime -v`
Expected: FAIL — `bootRuntime` / `startHookPath` undefined.

- [ ] **Step 3: Write minimal implementation**

```go
// server/server.go — add imports
"os/signal"   // already present
"path/filepath"
"strings"

"github.com/cryptowizard0/vmdocker_agent/supervisor"

// add fields to Server:
	sup           *supervisor.Supervisor
	startHookPath string

// in New(), default the hook path (matches the Dockerfile COPY target):
	startHookPath: getEnvOrDefault("VMDOCKER_USER_STARTUP_HOOK", "/usr/local/lib/vmdocker-agent/user-startup.sh"),

// new method:
func (s *Server) bootRuntime() error {
	env, err := s.launcher.Prepare()
	if err != nil {
		return fmt.Errorf("runtime prepare: %w", err)
	}
	for _, kv := range env {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			if err := os.Setenv(parts[0], parts[1]); err != nil {
				return fmt.Errorf("export env %s: %w", parts[0], err)
			}
		}
	}

	logPath := getEnvOrDefault("VMDOCKER_USER_STARTUP_LOG", "/tmp/vmdocker-user-startup.log")
	s.sup = supervisor.New(s.startHookPath, logPath)

	sigchld := make(chan os.Signal, 1)
	signal.Notify(sigchld, syscall.SIGCHLD)
	go s.sup.ReapLoop(sigchld)

	if err := s.sup.Start(); err != nil {
		return fmt.Errorf("start user startup hook: %w", err)
	}
	return nil
}
```

Update `Run` to boot the runtime, then forward `SIGTERM` to the engine on shutdown:

```go
func (s *Server) Run() error {
	log.Info("server running", "port", s.port)

	if err := s.bootRuntime(); err != nil {
		return fmt.Errorf("boot runtime: %w", err)
	}

	endpoint := fmt.Sprintf(":%d", s.port)
	go s.runAPI(endpoint)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	if s.sup != nil {
		if err := s.sup.Forward(syscall.SIGTERM); err != nil {
			log.Error("forward SIGTERM to engine failed", "err", err)
		}
	}
	return s.closeAPI()
}
```

Remove the now-unused `filepath` import if `startHookPath` default is a string literal — keep `filepath` only if used. (It is used by the test, not necessarily by server.go; drop it from server.go if `go build` complains.)

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/webbergao/work/src/HymxWorkspace/vmdocker_agent && go test ./server/ -v && go build ./...`
Expected: PASS; build clean.

- [ ] **Step 5: Commit**

```bash
cd /Users/webbergao/work/src/HymxWorkspace/vmdocker_agent
git add server/server.go server/api_test.go
git commit -m "feat(adapter): orchestrate prepare+spawn+reap+forward in Run"
```

---

### Task 6: Default start.sh templates

Ships the per-runtime default `start.sh`. openclaw backgrounds the gateway consuming the adapter-exported env; claude has no engine (init-only).

**Files:**
- Create: `startup/openclaw.sh`
- Create: `startup/claude.sh`
- Test: `startup/templates_test.go`

**Interfaces:**
- Produces: two POSIX `sh` scripts. openclaw reads `OPENCLAW_GATEWAY_LOG_PATH` (exported by the adapter) and optional `OPENCLAW_GATEWAY_PORT` / `_BIND` / `_TOKEN` / `_PASSWORD`.

- [ ] **Step 1: Write the failing test**

```go
// startup/templates_test.go
package startup_test

import (
	"os"
	"os/exec"
	"testing"
)

func TestOpenclawTemplateIsValidShell(t *testing.T) {
	if _, err := os.Stat("openclaw.sh"); err != nil {
		t.Fatalf("openclaw.sh missing: %v", err)
	}
	if out, err := exec.Command("sh", "-n", "openclaw.sh").CombinedOutput(); err != nil {
		t.Fatalf("openclaw.sh not valid sh: %v\n%s", err, out)
	}
}

func TestClaudeTemplateIsValidShell(t *testing.T) {
	if _, err := os.Stat("claude.sh"); err != nil {
		t.Fatalf("claude.sh missing: %v", err)
	}
	if out, err := exec.Command("sh", "-n", "claude.sh").CombinedOutput(); err != nil {
		t.Fatalf("claude.sh not valid sh: %v\n%s", err, out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/webbergao/work/src/HymxWorkspace/vmdocker_agent && go test ./startup/ -v`
Expected: FAIL — files missing.

- [ ] **Step 3: Write minimal implementation**

`startup/openclaw.sh`:

```sh
#!/bin/sh
# Default openclaw start.sh (platform template).
#
# The adapter has already run PrepareOpenclawRuntime and exported
# OPENCLAW_STATE_DIR / OPENCLAW_CONFIG_PATH / OPENCLAW_GATEWAY_LOG_PATH before
# invoking this script. This template starts the gateway in the background and
# returns; the adapter (PID 1) supervises it and gates /vmm/health on it.
#
# Copy and edit this file in your module to customize engine startup.
set -eu

PORT="${OPENCLAW_GATEWAY_PORT:-18789}"
BIND="${OPENCLAW_GATEWAY_BIND:-loopback}"
LOG="${OPENCLAW_GATEWAY_LOG_PATH:-/tmp/openclaw-gateway.log}"

set -- openclaw gateway --bind "${BIND}" --port "${PORT}" --allow-unconfigured
if [ -n "${OPENCLAW_GATEWAY_TOKEN:-}" ]; then
    set -- "$@" --auth token --token "${OPENCLAW_GATEWAY_TOKEN}"
elif [ -n "${OPENCLAW_GATEWAY_PASSWORD:-}" ]; then
    set -- "$@" --auth password --password "${OPENCLAW_GATEWAY_PASSWORD}"
fi

"$@" >"${LOG}" 2>&1 &

# Add module-specific initialization / workspace seeding below this line.
```

`startup/claude.sh`:

```sh
#!/bin/sh
# Default claude start.sh (platform template).
#
# The claude runtime has no long-running engine to start: the claude CLI is
# provided by the base image and /vmm/health becomes ready once it is on PATH.
# Add module-specific initialization below and return.
#
# Copy and edit this file in your module to customize startup.
set -eu

# Add module-specific initialization / workspace seeding below this line.
exit 0
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/webbergao/work/src/HymxWorkspace/vmdocker_agent && go test ./startup/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/webbergao/work/src/HymxWorkspace/vmdocker_agent
chmod +x startup/openclaw.sh startup/claude.sh
git add startup/openclaw.sh startup/claude.sh startup/templates_test.go
git commit -m "feat(adapter): add default openclaw/claude start.sh templates"
```

---

### Task 7: Delete wrapper, bootstrap hooks, isolation test

Removes the shell layers the adapter now replaces.

**Files:**
- Delete: `start-vmdocker-agent.sh`, `bootstrap/openclaw.sh`, `bootstrap/claude.sh`, `scripts/test_startup_hook_isolation.sh`

- [ ] **Step 1: Confirm no Go/code references remain**

Run:
```bash
cd /Users/webbergao/work/src/HymxWorkspace/vmdocker_agent
grep -rn "start-vmdocker-agent\|bootstrap/openclaw\|bootstrap/claude\|test_startup_hook_isolation\|VMDOCKER_WRAPPER_LIB\|BOOTSTRAP_DIR" --include="*.go" .
```
Expected: no output (Dockerfiles are updated in Task 8; only Go code is checked here).

- [ ] **Step 2: Delete the files**

```bash
cd /Users/webbergao/work/src/HymxWorkspace/vmdocker_agent
git rm start-vmdocker-agent.sh bootstrap/openclaw.sh bootstrap/claude.sh scripts/test_startup_hook_isolation.sh
```

- [ ] **Step 3: Verify build + full test suite still pass**

Run: `cd /Users/webbergao/work/src/HymxWorkspace/vmdocker_agent && go build ./... && go test ./...`
Expected: build clean; tests PASS.

- [ ] **Step 4: Commit**

```bash
cd /Users/webbergao/work/src/HymxWorkspace/vmdocker_agent
git commit -m "chore(adapter): remove wrapper, bootstrap hooks, isolation test"
```

---

### Task 8: Update vmdocker_agent base Dockerfiles

Point ENTRYPOINT at the adapter, drop the wrapper + bootstrap COPY, and COPY the matching default `start.sh` template.

**Files:**
- Modify: `Dockerfile.openclaw`, `Dockerfile.claude`, `Dockerfile.telegramcustomer`

- [ ] **Step 1: Read each Dockerfile and locate the wrapper/bootstrap/ENTRYPOINT lines**

Run: `cd /Users/webbergao/work/src/HymxWorkspace/vmdocker_agent && grep -n "start-vmdocker-agent\|bootstrap\|ENTRYPOINT\|user-startup\|/app/main" Dockerfile.openclaw Dockerfile.claude Dockerfile.telegramcustomer`
Expected: shows the COPY + ENTRYPOINT lines to change in each.

- [ ] **Step 2: Edit `Dockerfile.openclaw`**

Replace the wrapper COPY + bootstrap COPY + wrapper ENTRYPOINT with:
```dockerfile
COPY startup/openclaw.sh /usr/local/lib/vmdocker-agent/user-startup.sh
```
and set the final line to:
```dockerfile
ENTRYPOINT ["/app/main"]
```
Remove: `COPY start-vmdocker-agent.sh ...`, `COPY bootstrap/ ...`, and any `test -x .../bootstrap/openclaw.sh` guard. In the `chmod`/`chown` RUN, replace references to `start-vmdocker-agent.sh` and `bootstrap/*.sh` with `/usr/local/lib/vmdocker-agent/user-startup.sh`. Keep `ENV RUNTIME_TYPE=openclaw` and `USER agent`/`USER hymx`.

- [ ] **Step 3: Edit `Dockerfile.claude`**

Same as Step 2 but COPY `startup/claude.sh`:
```dockerfile
COPY startup/claude.sh /usr/local/lib/vmdocker-agent/user-startup.sh
```
ENTRYPOINT `["/app/main"]`; keep `ENV RUNTIME_TYPE=claude`. Remove the `bootstrap/claude.sh` COPY + guard + wrapper COPY.

- [ ] **Step 4: Edit `Dockerfile.telegramcustomer`**

Same pattern. There is no telegramcustomer engine template, so COPY the claude template (init-only default) as the hook, or a minimal inline `exit 0` script if you prefer; keep `ENV RUNTIME_TYPE=telegramcustomer`, ENTRYPOINT `["/app/main"]`, drop wrapper + bootstrap.
```dockerfile
COPY startup/claude.sh /usr/local/lib/vmdocker-agent/user-startup.sh
```

- [ ] **Step 5: Verify no wrapper/bootstrap references remain**

Run: `cd /Users/webbergao/work/src/HymxWorkspace/vmdocker_agent && grep -n "start-vmdocker-agent\|COPY bootstrap" Dockerfile.openclaw Dockerfile.claude Dockerfile.telegramcustomer`
Expected: no output.

- [ ] **Step 6: Commit**

```bash
cd /Users/webbergao/work/src/HymxWorkspace/vmdocker_agent
git add Dockerfile.openclaw Dockerfile.claude Dockerfile.telegramcustomer
git commit -m "build(adapter): launch adapter directly as ENTRYPOINT, drop wrapper"
```

---

# Phase 2 — vmdockerv2 host

### Task 9: Enforce non-root User and disable swap

Host-level enforcement in the container/host config: run as the non-root `hymx` uid regardless of the image's `USER`, and pin `MemorySwap` to `Memory`.

**Files:**
- Modify: `vmdocker/runtimemanager/docker.go` (`buildDockerContainerConfig`, `buildDockerHostConfig`)
- Test: `vmdocker/runtimemanager/docker_test.go`

**Interfaces:**
- Consumes: existing `schema.MaxMem`, `containerHome`.
- Produces: `container.Config.User == schema.RuntimeUser`; `hostConfig.Resources.MemorySwap == int64(schema.MaxMem)`. New const `schema.RuntimeUser = "hymx"`.

- [ ] **Step 1: Write the failing test**

```go
// vmdocker/runtimemanager/docker_test.go
package runtimemanager

import (
	"testing"

	"github.com/cryptowizard0/vmdockerv2/vmdocker/runtimemanager/schema"
)

func TestBuildDockerHostConfigPinsSwapToMemory(t *testing.T) {
	hc := buildDockerHostConfig(12345, t.TempDir())
	if hc.Resources.MemorySwap != int64(schema.MaxMem) {
		t.Fatalf("MemorySwap = %d, want %d (== Memory)", hc.Resources.MemorySwap, int64(schema.MaxMem))
	}
}

func TestBuildDockerContainerConfigRunsNonRoot(t *testing.T) {
	spec := schema.RuntimeSpec{Image: schema.ImageSpec{Name: "img"}}
	cfg, err := buildDockerContainerConfig(spec, nil, []string{"/app/main"}, t.TempDir())
	if err != nil {
		t.Fatalf("build config: %v", err)
	}
	if cfg.User != schema.RuntimeUser {
		t.Fatalf("User = %q, want %q", cfg.User, schema.RuntimeUser)
	}
	if schema.RuntimeUser == "" || schema.RuntimeUser == "root" || schema.RuntimeUser == "0" {
		t.Fatalf("RuntimeUser must be a non-root user, got %q", schema.RuntimeUser)
	}
}
```

Note: confirm the exact `schema` import path and `ImageSpec`/`RuntimeSpec` field names against `vmdocker/runtimemanager/schema/schema.go` and adjust the literal in the test if needed.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/webbergao/work/src/HymxWorkspace/vmdockerv2 && go test ./vmdocker/runtimemanager/ -run 'TestBuildDocker' -v`
Expected: FAIL — `schema.RuntimeUser` undefined; `MemorySwap` is `-1`.

- [ ] **Step 3: Write minimal implementation**

Add the const in `vmdocker/runtimemanager/schema/env.go` (next to `MaxMem`):

```go
	// RuntimeUser is the non-root user the runtime container is forced to run
	// as, matching the user the generated Dockerfile creates.
	RuntimeUser = "hymx"
```

In `docker.go`, `buildDockerHostConfig` Resources — change swap:

```go
		Resources: container.Resources{
			Memory:     int64(schema.MaxMem),
			MemorySwap: int64(schema.MaxMem), // == Memory: no swap dilution
			PidsLimit:  &pidsLimit,
			CPUPeriod:  100000,
			CPUQuota:   200000,
			CPUShares:  1024,
		},
```

In `docker.go`, `buildDockerContainerConfig` return — add `User`:

```go
	return &container.Config{
		Image:      runtimeSpec.Image.Name,
		User:       schema.RuntimeUser,
		Entrypoint: []string{startCommand[0]},
		ExposedPorts: nat.PortSet{
			nat.Port(schema.ExprotPort): struct{}{},
		},
		Cmd:        startCommand[1:],
		Env:        runtimeEnv,
		WorkingDir: containerHome,
	}, nil
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/webbergao/work/src/HymxWorkspace/vmdockerv2 && go test ./vmdocker/runtimemanager/ -run 'TestBuildDocker' -v && go build ./...`
Expected: PASS; build clean.

- [ ] **Step 5: Commit**

```bash
cd /Users/webbergao/work/src/HymxWorkspace/vmdockerv2
git add vmdocker/runtimemanager/docker.go vmdocker/runtimemanager/schema/env.go vmdocker/runtimemanager/docker_test.go
git commit -m "feat(host): enforce non-root User and pin MemorySwap to Memory"
```

---

### Task 10: Node-startup confinement self-check

A one-shot check against the local Docker daemon (`cli.Info()`) that verifies the confinement the host config requests will actually take effect. Emits a pass/warn/fail report; refuses or warns per policy.

**Files:**
- Create: `vmdocker/runtimemanager/nodecheck.go`
- Test: `vmdocker/runtimemanager/nodecheck_test.go`

**Interfaces:**
- Consumes: `github.com/docker/docker/api/types/system` (`system.Info` with `MemoryLimit`, `SwapLimit`, `PidsLimit bool`, `SecurityOptions []string`, `ServerVersion string`).
- Produces:
  - `type infoClient interface { Info(context.Context) (system.Info, error) }`
  - `type CheckResult struct { Name string; OK bool; Severity string; Detail string }` (Severity: `"refuse"`, `"warn"`, `"info"`)
  - `func RunNodeConfinementCheck(ctx context.Context, cli infoClient, strict bool) ([]CheckResult, error)` — returns the report; returns a non-nil error if any `refuse`-severity check fails (strict mode also fails on `warn`).

- [ ] **Step 1: Write the failing test**

```go
// vmdocker/runtimemanager/nodecheck_test.go
package runtimemanager

import (
	"context"
	"testing"

	"github.com/docker/docker/api/types/system"
)

type fakeInfoClient struct {
	info system.Info
	err  error
}

func (f fakeInfoClient) Info(context.Context) (system.Info, error) { return f.info, f.err }

func healthyInfo() system.Info {
	return system.Info{
		ServerVersion:   "28.0.4",
		MemoryLimit:     true,
		SwapLimit:       true,
		PidsLimit:       true,
		SecurityOptions: []string{"name=seccomp,profile=builtin", "name=apparmor"},
	}
}

func TestNodeCheckHealthyPasses(t *testing.T) {
	report, err := RunNodeConfinementCheck(context.Background(), fakeInfoClient{info: healthyInfo()}, false)
	if err != nil {
		t.Fatalf("healthy node should pass, got %v", err)
	}
	for _, r := range report {
		if r.Severity == "refuse" && !r.OK {
			t.Fatalf("refuse check failed on healthy node: %s (%s)", r.Name, r.Detail)
		}
	}
}

func TestNodeCheckSeccompMissingRefuses(t *testing.T) {
	info := healthyInfo()
	info.SecurityOptions = []string{"name=apparmor"} // no seccomp
	_, err := RunNodeConfinementCheck(context.Background(), fakeInfoClient{info: info}, false)
	if err == nil {
		t.Fatal("missing seccomp must refuse, got nil error")
	}
}

func TestNodeCheckMemoryLimitMissingRefuses(t *testing.T) {
	info := healthyInfo()
	info.MemoryLimit = false
	_, err := RunNodeConfinementCheck(context.Background(), fakeInfoClient{info: info}, false)
	if err == nil {
		t.Fatal("missing memory limit must refuse, got nil error")
	}
}

func TestNodeCheckStrictEscalatesWarn(t *testing.T) {
	info := healthyInfo()
	info.SwapLimit = false // warn by default
	if _, err := RunNodeConfinementCheck(context.Background(), fakeInfoClient{info: info}, false); err != nil {
		t.Fatalf("non-strict should tolerate swap warn, got %v", err)
	}
	if _, err := RunNodeConfinementCheck(context.Background(), fakeInfoClient{info: info}, true); err == nil {
		t.Fatal("strict mode must escalate swap warn to failure")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/webbergao/work/src/HymxWorkspace/vmdockerv2 && go test ./vmdocker/runtimemanager/ -run TestNodeCheck -v`
Expected: FAIL — `RunNodeConfinementCheck` undefined.

- [ ] **Step 3: Write minimal implementation**

```go
// vmdocker/runtimemanager/nodecheck.go
package runtimemanager

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/system"
)

type infoClient interface {
	Info(context.Context) (system.Info, error)
}

// CheckResult is one confinement-capability finding.
type CheckResult struct {
	Name     string
	OK       bool
	Severity string // "refuse", "warn", "info"
	Detail   string
}

func hasSecurityOption(opts []string, name string) bool {
	for _, o := range opts {
		if strings.Contains(o, "name="+name) {
			return true
		}
	}
	return false
}

// RunNodeConfinementCheck inspects the local Docker daemon once and reports
// whether the confinement the host config requests will take effect. It returns
// an error if any refuse-severity check fails (in strict mode, warn fails too).
func RunNodeConfinementCheck(ctx context.Context, cli infoClient, strict bool) ([]CheckResult, error) {
	info, err := cli.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("docker daemon unreachable: %w", err)
	}

	report := []CheckResult{
		{Name: "daemon-version", OK: info.ServerVersion != "", Severity: "refuse",
			Detail: "ServerVersion=" + info.ServerVersion},
		{Name: "seccomp", OK: hasSecurityOption(info.SecurityOptions, "seccomp"), Severity: "refuse",
			Detail: "seccomp default profile"},
		{Name: "memory-limit", OK: info.MemoryLimit, Severity: "refuse",
			Detail: "HostConfig.Memory enforceable"},
		{Name: "swap-limit", OK: info.SwapLimit, Severity: "warn",
			Detail: "MemorySwap==Memory enforceable"},
		{Name: "pids-limit", OK: info.PidsLimit, Severity: "warn",
			Detail: "PidsLimit enforceable"},
		{Name: "mac", OK: hasSecurityOption(info.SecurityOptions, "apparmor") ||
			hasSecurityOption(info.SecurityOptions, "selinux"), Severity: "warn",
			Detail: "AppArmor or SELinux present"},
	}

	var failures []string
	for _, r := range report {
		if r.OK {
			log.Info("node confinement check", "name", r.Name, "ok", true, "detail", r.Detail)
			continue
		}
		fatal := r.Severity == "refuse" || (strict && r.Severity == "warn")
		log.Warn("node confinement check", "name", r.Name, "severity", r.Severity, "fatal", fatal, "detail", r.Detail)
		if fatal {
			failures = append(failures, r.Name)
		}
	}

	if len(failures) > 0 {
		return report, fmt.Errorf("node confinement check failed: %s", strings.Join(failures, ", "))
	}
	return report, nil
}
```

Note: `log` is the package logger already declared in `vmdocker/runtimemanager` (used across `docker.go`/`sandbox.go`). Confirm its name; if the package uses a different logger symbol, match it.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/webbergao/work/src/HymxWorkspace/vmdockerv2 && go test ./vmdocker/runtimemanager/ -run TestNodeCheck -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/webbergao/work/src/HymxWorkspace/vmdockerv2
git add vmdocker/runtimemanager/nodecheck.go vmdocker/runtimemanager/nodecheck_test.go
git commit -m "feat(host): add node-startup Docker confinement self-check"
```

---

### Task 11: Call the self-check at node startup

Wires `RunNodeConfinementCheck` into the `DockerManager` constructor, once, so a node with inadequate confinement is caught before spawning.

**Files:**
- Modify: `vmdocker/runtimemanager/docker.go` (the `DockerManager` constructor)
- Test: `vmdocker/runtimemanager/nodecheck_test.go` (extend)

**Interfaces:**
- Consumes: `RunNodeConfinementCheck` (Task 10); `*client.Client` satisfies `infoClient`.
- Produces: node check invoked in the constructor guarded by `sync.Once`; a `refuse`-severity failure makes the constructor return an error (refuse by default — see Step 3).

- [ ] **Step 1: Read the constructor to find the exact name and signature**

Run: `cd /Users/webbergao/work/src/HymxWorkspace/vmdockerv2 && grep -n "func New.*DockerManager\|client.NewClientWithOpts" vmdocker/runtimemanager/docker.go`
Expected: reveals the constructor (e.g. `NewDockerManager(...)`) where `cli` is created.

- [ ] **Step 2: Write the failing test**

```go
// append to vmdocker/runtimemanager/nodecheck_test.go
func TestNodeCheckReportIncludesAllExpectedChecks(t *testing.T) {
	report, _ := RunNodeConfinementCheck(context.Background(), fakeInfoClient{info: healthyInfo()}, false)
	want := map[string]bool{
		"daemon-version": false, "seccomp": false, "memory-limit": false,
		"swap-limit": false, "pids-limit": false, "mac": false,
	}
	for _, r := range report {
		want[r.Name] = true
	}
	for name, seen := range want {
		if !seen {
			t.Fatalf("report missing check %q", name)
		}
	}
}
```

(This locks the report shape the constructor logs; the constructor wiring itself is verified by build + the daemon-integration run in Step 4.)

- [ ] **Step 3: Add the call in the constructor**

In `docker.go`, after `cli` is successfully created in the constructor, add (using the real constructor/return-var names found in Step 1):

```go
	nodeCheckOnce.Do(func() {
		strict := os.Getenv("VMDOCKER_NODE_CHECK_STRICT") == "1"
		_, nodeCheckErr = RunNodeConfinementCheck(context.Background(), cli, strict)
	})
	if nodeCheckErr != nil {
		return nil, fmt.Errorf("node confinement check failed: %w", nodeCheckErr)
	}
```

Add package-level vars `var nodeCheckOnce sync.Once` and `var nodeCheckErr error` (package-level so the result persists across constructor calls under `sync.Once`), and ensure `os`, `context`, `sync`, `fmt` are imported.

Policy (owner-confirmed): **refuse by default.** `RunNodeConfinementCheck` already returns an error on any refuse-severity failure (seccomp, memory-limit, daemon-version) regardless of `strict`, so the constructor refuses node startup on those by default. `VMDOCKER_NODE_CHECK_STRICT=1` additionally escalates warn-severity checks (swap/pids/MAC) to refusal. This matches spec §6.1.

- [ ] **Step 4: Run tests + build**

Run: `cd /Users/webbergao/work/src/HymxWorkspace/vmdockerv2 && go test ./vmdocker/runtimemanager/ -v && go build ./...`
Expected: PASS; build clean.

- [ ] **Step 5: Commit**

```bash
cd /Users/webbergao/work/src/HymxWorkspace/vmdockerv2
git add vmdocker/runtimemanager/docker.go vmdocker/runtimemanager/nodecheck_test.go
git commit -m "feat(host): run confinement self-check once at manager startup"
```

---

### Task 12: modulebuild — drop wrapper, keep RUNTIME_TYPE

Update the generated Dockerfile: ENTRYPOINT = adapter binary, remove the wrapper COPY/field, keep `ENV RUNTIME_TYPE`, keep the `startup` COPY.

**Files:**
- Modify: `vmdocker/modulebuild/dockerfile.go`
- Modify: `vmdocker/modulebuild/dockerfile_test.go`
- Modify: `vmdocker/modulebuild/build.go`
- Modify: `vmdocker/modulebuild/build_test.go`

**Interfaces:**
- Produces: `DockerfileInput` loses `WrapperSrc`; `GenerateDockerfile` no longer requires it; generated `ENTRYPOINT ["/usr/local/bin/vmdocker-agent"]`; `ENV RUNTIME_TYPE={{.RuntimeType}}` retained. `BuildOptions` loses `WrapperPath`; `stageBuildContext` no longer stages the wrapper.

- [ ] **Step 1: Write the failing test**

```go
// vmdocker/modulebuild/dockerfile_test.go — add/replace assertions
func TestGenerateDockerfileUsesAdapterEntrypoint(t *testing.T) {
	profile := Profile{Dockerfile: DockerfileSection{
		From: "openclaw", Bin: "bin", Startup: "start.sh",
	}}
	out, err := GenerateDockerfile(DockerfileInput{
		Profile:     profile,
		AgentBinSrc: "platform/vmdocker-agent",
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(out, `ENTRYPOINT ["/usr/local/bin/vmdocker-agent"]`) {
		t.Fatalf("ENTRYPOINT not adapter:\n%s", out)
	}
	if strings.Contains(out, "start-vmdocker-agent.sh") {
		t.Fatalf("wrapper still referenced:\n%s", out)
	}
	if !strings.Contains(out, "ENV RUNTIME_TYPE=openclaw") {
		t.Fatalf("RUNTIME_TYPE dropped:\n%s", out)
	}
	if !strings.Contains(out, "user-startup.sh") {
		t.Fatalf("startup COPY dropped:\n%s", out)
	}
}
```

Remove any existing test asserting the wrapper ENTRYPOINT or requiring `WrapperSrc`.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/webbergao/work/src/HymxWorkspace/vmdockerv2 && go test ./vmdocker/modulebuild/ -run TestGenerateDockerfile -v`
Expected: FAIL — ENTRYPOINT still the wrapper; unknown field if `WrapperSrc` removed from struct literal.

- [ ] **Step 3: Edit `dockerfile.go`**

- In the template string, change the last line to `ENTRYPOINT ["/usr/local/bin/vmdocker-agent"]`.
- Remove the `COPY {{.WrapperSrc}} /usr/local/bin/start-vmdocker-agent.sh` line.
- In the `chmod` RUN, drop `start-vmdocker-agent.sh` from the list (keep `user-startup.sh`).
- Remove `WrapperSrc` from `DockerfileInput` and `dockerfileView`.
- In `GenerateDockerfile`, drop the `WrapperSrc` from the required-args check (keep `AgentBinSrc`), and remove `WrapperSrc:` from the `dockerfileView{...}` literal.
- Keep `ENV RUNTIME_TYPE={{.RuntimeType}}`.

- [ ] **Step 4: Edit `build.go`**

- Remove `WrapperPath` from `BuildOptions`.
- In `stageBuildContext`, remove the `WrapperSrc:` arg from the `GenerateDockerfile` call and delete the block that copies `opts.WrapperPath` to `platform/start-vmdocker-agent.sh`. Keep staging `platform/vmdocker-agent`.

Update `build_test.go`: remove any `WrapperPath:` fields from `BuildOptions{...}` literals.

- [ ] **Step 5: Run tests + build**

Run: `cd /Users/webbergao/work/src/HymxWorkspace/vmdockerv2 && go test ./vmdocker/modulebuild/ -v && go build ./...`
Expected: PASS; build clean.

- [ ] **Step 6: Commit**

```bash
cd /Users/webbergao/work/src/HymxWorkspace/vmdockerv2
git add vmdocker/modulebuild/dockerfile.go vmdocker/modulebuild/dockerfile_test.go vmdocker/modulebuild/build.go vmdocker/modulebuild/build_test.go
git commit -m "feat(modulebuild): adapter ENTRYPOINT, drop wrapper, keep RUNTIME_TYPE"
```

---

### Task 13: Full-suite verification

Confirm both repos build and pass end-to-end after the change.

- [ ] **Step 1: vmdocker_agent**

Run: `cd /Users/webbergao/work/src/HymxWorkspace/vmdocker_agent && go build ./... && go vet ./... && go test ./...`
Expected: all PASS.

- [ ] **Step 2: vmdockerv2**

Run: `cd /Users/webbergao/work/src/HymxWorkspace/vmdockerv2 && go build ./... && go vet ./... && go test ./...`
Expected: all PASS.

- [ ] **Step 3: Grep for orphaned references across both repos**

Run:
```bash
grep -rn "start-vmdocker-agent\|WrapperSrc\|WrapperPath\|bootstrap/openclaw\|bootstrap/claude" \
  /Users/webbergao/work/src/HymxWorkspace/vmdocker_agent \
  /Users/webbergao/work/src/HymxWorkspace/vmdockerv2 \
  --include="*.go" --include="Dockerfile*" | grep -v "/.git/"
```
Expected: no output.

- [ ] **Step 4: Commit any cleanup**

```bash
# only if Step 3 surfaced stragglers you fixed
cd /Users/webbergao/work/src/HymxWorkspace/vmdockerv2 && git add -A && git commit -m "chore: remove orphaned wrapper references"
```

---

## Self-Review Notes

- **Spec coverage:** §6.1 host enforcement → Tasks 9–11; §6.2 adapter (prep/env, spawn/supervise, health gate, PID 1) → Tasks 1–5; §6.3 start.sh templates → Task 6; §8.1 modulebuild → Task 12; §8.2 deletions + base Dockerfiles → Tasks 7–8; AC#1–8 each map to a task (AC#1/#8 → Tasks 8+12; AC#2/#3 → Task 9; AC#4 → Tasks 1+4+6; AC#5/#6 → Tasks 2–5; AC#7 → Tasks 10–11).
- **RUNTIME_TYPE retained** everywhere (global constraint), verified by the Task 12 assertion.
- **Portability:** PID 1 reaping is tested via the injectable `reap` seam (Task 3), not real subreaper semantics, so tests pass on the darwin dev machine.
- **Confirm-before-code items flagged inline:** exact `schema` field names (Task 9), package logger symbol (Task 10), `DockerManager` constructor name and default refuse-vs-warn policy (Task 11).
