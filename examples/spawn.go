package main

import (
	"fmt"
)

func spawn() {
	runtimeBackend := GetEnvWith("RUNTIME_BACKEND", "")
	res, err := s.Spawn(
		module,
		scheduler,
		runtimeBackendTags(runtimeBackend),
	)
	fmt.Printf("res: %#v, err: %v\n", res, err)
}
