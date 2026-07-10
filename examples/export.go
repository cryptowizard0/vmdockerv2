package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	vmmSchema "github.com/hymatrix/hymx/vmm/schema"
	"github.com/permadao/goar/schema"
)

// exportProcess sends an Export capability message to a running vmdockerv2
// process. Export reuses the agent's existing image as-is and folds the
// process's current public files (profile [vmdocker].public allowlist) into the
// new module's public.zip — the program is carried over unchanged, only the
// public state is re-captured. The node writes the resulting module into its own
// module store (mod/mod-<id>.json) and returns just the module id, so the full
// container image never travels back through the result channel.
//
// Usage:
//
//	go run ./examples export <pid>
//
// or set VMDOCKER_EXPORT_PID in .env and run `go run ./examples export`.
// Set VMDOCKER_EXPORT_DRY_RUN=1 to preview the public collection without
// producing a module.
func exportProcess() {
	pid := exportTargetPID()
	if pid == "" {
		fmt.Println("usage: go run ./examples export <pid>   (or set VMDOCKER_EXPORT_PID in .env)")
		os.Exit(1)
	}

	tags := []schema.Tag{{Name: "Action", Value: "Export"}}
	if GetEnvWith("VMDOCKER_EXPORT_DRY_RUN", "") != "" {
		tags = append(tags, schema.Tag{Name: "Dry-Run", Value: "true"})
	}

	fmt.Printf("exporting process %s ...\n", pid)
	res, err := s.SendMessageAndWait(pid, "", tags)
	if err != nil {
		fmt.Printf("export message failed: %v\n", err)
		os.Exit(1)
	}

	// SendMessageAndWait returns the process's VmmResult marshaled into res.Message.
	var result vmmSchema.VmmResult
	if err := json.Unmarshal([]byte(res.Message), &result); err != nil {
		fmt.Printf("decode export result failed: %v\nraw: %s\n", err, res.Message)
		os.Exit(1)
	}
	if result.Error != "" {
		fmt.Printf("export failed on node: %s\n", result.Error)
		os.Exit(1)
	}

	// Output carries the public.zip collection preview (files that were exported).
	if result.Output != nil {
		if out, err := json.MarshalIndent(result.Output, "", "  "); err == nil {
			fmt.Printf("exported public collection:\n%s\n", out)
		}
	}

	// Data is the exported module id; the node has already written
	// mod-<id>.json into its module store. Empty on a dry-run.
	moduleID := strings.TrimSpace(result.Data)
	if moduleID == "" {
		fmt.Println("dry-run: preview only, no module written")
		return
	}

	fmt.Printf("exported module id: %s\n", moduleID)
	fmt.Printf("the node wrote mod-%s.json into its module store\n", moduleID)
	fmt.Printf("re-spawn it: set VMDOCKER_MODULE_ID=%s in .env, then run `go run ./examples spawn`\n", moduleID)
}

func exportTargetPID() string {
	if len(os.Args) >= 3 {
		return os.Args[2]
	}
	return GetEnvWith("VMDOCKER_EXPORT_PID", "")
}
