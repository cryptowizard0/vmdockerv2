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
