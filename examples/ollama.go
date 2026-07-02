package main

import (
	"fmt"

	serverSchema "github.com/hymatrix/hymx/server/schema"
	"github.com/permadao/goar/schema"
	goarSchema "github.com/permadao/goar/schema"
)

func ollama() {
	var res *serverSchema.Response
	var err error

	res, err = s.SpawnAndWait(
		"LSjhdzBjyWuyUPe-g6PUzt8t1PUlw2FZ9SM3_hCh2Is",
		"0x972AeD684D6f817e1b58AF70933dF1b4a75bfA51",
		[]goarSchema.Tag{
			{Name: "Module-Format", Value: "ollama"},
			// {Name: "RuntimeType", Value: "golua"},
		},
	)
	if err != nil {
		fmt.Println("Failed to spawn: ", err)
		return
	}
	fmt.Println("spawn success: ", res)

	target := res.Id

	res, err = s.SendMessageAndWait(target, "",
		[]schema.Tag{
			{Name: "Action", Value: "Chat"},
			{Name: "Prompt", Value: "hello"},
			{Name: "Target", Value: target},
		})
	if err != nil {
		fmt.Println("handler error: ", err)
		return
	}
	fmt.Println("target1 load ok, ", res)
}
