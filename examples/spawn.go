package main

import (
	"fmt"
)

func spawn() {
	runtimeBackend := GetEnvWith("RUNTIME_BACKEND", "")
	res, err := s.SpawnAndWait(
		module,
		scheduler,
		runtimeBackendTags(runtimeBackend),
	)
	if err != nil {
		fmt.Printf("spawn failed: %v\n", err)
		return
	}
	fmt.Printf("spawned pid: %s\n", res.Id)
}
