package runtimemanager

import (
	"context"
	"fmt"
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

func TestNodeCheckSeccompUnconfinedRefuses(t *testing.T) {
	info := healthyInfo()
	// Daemon reports seccomp present but with the unconfined profile — seccomp is
	// effectively disabled for containers launched without a per-container
	// profile (which vmdocker never sets). This must refuse, not pass.
	info.SecurityOptions = []string{"name=seccomp,profile=unconfined", "name=apparmor"}
	_, err := RunNodeConfinementCheck(context.Background(), fakeInfoClient{info: info}, false)
	if err == nil {
		t.Fatal("seccomp profile=unconfined must refuse, got nil error")
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

// flakyInfoClient fails the first failFirst Info() calls, then returns info.
type flakyInfoClient struct {
	calls     int
	failFirst int
	info      system.Info
}

func (f *flakyInfoClient) Info(context.Context) (system.Info, error) {
	f.calls++
	if f.calls <= f.failFirst {
		return system.Info{}, fmt.Errorf("daemon temporarily unreachable")
	}
	return f.info, nil
}

func TestCheckNodeConfinementRetriesTransientFailure(t *testing.T) {
	dm := &DockerManager{}
	fake := &flakyInfoClient{failFirst: 1, info: healthyInfo()}

	// First spawn: daemon momentarily down. Must error but NOT latch.
	if err := dm.checkNodeConfinement(fake); err == nil {
		t.Fatal("transient daemon failure should return an error")
	}
	if dm.nodeChecked {
		t.Fatal("transient failure must not latch the gate")
	}

	// Next spawn: daemon recovered. Must pass and latch.
	if err := dm.checkNodeConfinement(fake); err != nil {
		t.Fatalf("check should pass once daemon recovers, got %v", err)
	}
	if !dm.nodeChecked {
		t.Fatal("successful verdict should latch")
	}

	// Subsequent spawns must reuse the latched pass without re-querying Info().
	callsBefore := fake.calls
	if err := dm.checkNodeConfinement(fake); err != nil {
		t.Fatalf("latched pass should stay passing, got %v", err)
	}
	if fake.calls != callsBefore {
		t.Fatalf("latched gate must not re-invoke Info(): calls %d -> %d", callsBefore, fake.calls)
	}
}

func TestCheckNodeConfinementLatchesRealVerdict(t *testing.T) {
	dm := &DockerManager{}
	info := healthyInfo()
	info.SecurityOptions = []string{"name=apparmor"} // seccomp missing -> refuse verdict
	fake := &flakyInfoClient{info: info}

	if err := dm.checkNodeConfinement(fake); err == nil {
		t.Fatal("misconfigured node must fail")
	}
	if !dm.nodeChecked {
		t.Fatal("a real (non-transient) verdict must latch, even when failing")
	}
	callsBefore := fake.calls
	if err := dm.checkNodeConfinement(fake); err == nil {
		t.Fatal("latched failing verdict must keep failing")
	}
	if fake.calls != callsBefore {
		t.Fatalf("latched failing verdict must not re-invoke Info(): calls %d -> %d", callsBefore, fake.calls)
	}
}

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
