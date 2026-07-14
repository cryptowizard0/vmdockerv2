package main

import (
	"fmt"
)

func spawn() {
	tags := runtimeBackendTags(GetEnvWith("RUNTIME_BACKEND", ""))
	tags = append(tags, runtimeTypeTags(GetEnvWith("RUNTIME_TYPE", ""))...)
	res, err := s.SpawnAndWait(
		module,
		scheduler,
		tags,
	)
	if err != nil {
		fmt.Printf("spawn failed: %v\n", err)
		return
	}
	fmt.Printf("spawned pid: %s\n", res.Id)
}
