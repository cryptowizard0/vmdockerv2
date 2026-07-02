package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func genModule() {
	agentRepo, err := findVMDockerAgentRepo()
	if err != nil {
		fmt.Printf("locate vmdocker_agent failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("module generation has moved to vmdocker_agent, delegating to %s\n", agentRepo)
	cmd := exec.Command("go", "run", "./cmd/module")
	cmd.Dir = agentRepo
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		fmt.Printf("run vmdocker_agent module generator failed: %v\n", err)
		os.Exit(1)
	}
}

func findVMDockerAgentRepo() (string, error) {
	if repo := os.Getenv("VMDOCKER_AGENT_DIR"); repo != "" {
		return filepath.Abs(repo)
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	candidates := []string{
		filepath.Join(wd, "../vmdocker_agent"),
		filepath.Join(wd, "../../vmdocker_agent"),
	}
	for _, candidate := range candidates {
		info, err := os.Stat(filepath.Join(candidate, "cmd", "module", "main.go"))
		if err == nil && !info.IsDir() {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("set VMDOCKER_AGENT_DIR to the vmdocker_agent repository path")
}
