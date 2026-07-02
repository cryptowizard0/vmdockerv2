package main

import goarSchema "github.com/permadao/goar/schema"

func runtimeBackendTags(runtimeBackend string) []goarSchema.Tag {
	if runtimeBackend == "" {
		return nil
	}
	return []goarSchema.Tag{
		{Name: "Runtime-Backend", Value: runtimeBackend},
	}
}
