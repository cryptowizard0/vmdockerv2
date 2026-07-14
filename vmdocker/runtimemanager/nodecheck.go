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

// seccompEffective reports whether the daemon's default seccomp profile will
// actually confine containers. Docker's Info reports the seccomp entry as
// "name=seccomp,profile=<profile>". A profile of "unconfined" means seccomp is
// disabled for every container launched without an explicit per-container
// profile — which vmdocker never sets — so it must NOT count as confined.
// Matching only "name=seccomp" (as hasSecurityOption does) would fail open here.
func seccompEffective(opts []string) bool {
	for _, o := range opts {
		if !strings.Contains(o, "name=seccomp") {
			continue
		}
		return !strings.Contains(o, "profile=unconfined")
	}
	return false
}

// RunNodeConfinementCheck inspects the local Docker daemon once and reports
// whether the confinement the host config requests will take effect. It returns
// an error if any refuse-severity check fails (in strict mode, warn fails too).
//
// The two error modes are distinguishable by the report: a nil report means the
// daemon was unreachable (a transient failure the caller may retry), while a
// non-nil report with an error means the confinement verdict itself failed (a
// persistent misconfiguration the caller may cache).
func RunNodeConfinementCheck(ctx context.Context, cli infoClient, strict bool) ([]CheckResult, error) {
	info, err := cli.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("docker daemon unreachable: %w", err)
	}

	report := []CheckResult{
		{Name: "daemon-version", OK: info.ServerVersion != "", Severity: "refuse",
			Detail: "ServerVersion=" + info.ServerVersion},
		{Name: "seccomp", OK: seccompEffective(info.SecurityOptions), Severity: "refuse",
			Detail: "seccomp default profile (not unconfined)"},
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
