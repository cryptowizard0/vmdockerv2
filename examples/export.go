package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	vmmSchema "github.com/hymatrix/hymx/vmm/schema"
	"github.com/permadao/goar/schema"
	goarSchema "github.com/permadao/goar/schema"
	arutils "github.com/permadao/goar/utils"
)

// exportProcess sends an Export capability message to a running vmdockerv2
// process and writes the returned signed module as mod-<id>.json, ready to be
// spawned again. Export reuses the agent's existing image as-is and folds the
// process's current public files (profile [vmdocker].public allowlist) into the
// new module's public.zip — the program is carried over unchanged, only the
// public state is re-captured.
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

	if result.Data == "" {
		fmt.Println("dry-run: preview only, no module written")
		return
	}

	// result.Data is a base64 (RawURL) signed module BundleItem JSON — the same
	// format the node loads from cmd/mod/mod-<id>.json.
	moduleJSON, err := arutils.Base64Decode(result.Data)
	if err != nil {
		fmt.Printf("decode module data failed: %v\n", err)
		os.Exit(1)
	}

	var item goarSchema.BundleItem
	if err := json.Unmarshal(moduleJSON, &item); err != nil {
		fmt.Printf("parse exported module item failed: %v\n", err)
		os.Exit(1)
	}

	outDir := GetEnvWith("VMDOCKER_EXPORT_OUT_DIR", filepath.Join("cmd", "mod"))
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Printf("create %s failed: %v\n", outDir, err)
		os.Exit(1)
	}
	outPath := filepath.Join(outDir, fmt.Sprintf("mod-%s.json", item.Id))
	if err := os.WriteFile(outPath, moduleJSON, 0o644); err != nil {
		fmt.Printf("write %s failed: %v\n", outPath, err)
		os.Exit(1)
	}

	fmt.Printf("exported module id: %s\n", item.Id)
	fmt.Printf("wrote %s (%d bytes)\n", outPath, len(moduleJSON))
	fmt.Printf("re-spawn it: set VMDOCKER_MODULE_ID=%s in .env, restart the node, then run `go run ./examples spawn`\n", item.Id)
}

func exportTargetPID() string {
	if len(os.Args) >= 3 {
		return os.Args[2]
	}
	return GetEnvWith("VMDOCKER_EXPORT_PID", "")
}
