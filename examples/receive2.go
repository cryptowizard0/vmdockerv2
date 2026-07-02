package main

import (
	"fmt"

	goarSchema "github.com/permadao/goar/schema"
)

func receive2() {
	// s := sdk.New("http://127.0.0.1:8080", "../test_keyfile.json")

	// spawn target1
	res, err := s.SpawnAndWait(
		module,
		scheduler,
		[]goarSchema.Tag{},
	)
	if err != nil {
		fmt.Println("Failed to spawn: ", err)
		return
	}
	target1 := res.Id
	fmt.Println("spawn target1: ", target1)

	// spawn target2
	res, err = s.SpawnAndWait(
		module,
		scheduler,
		[]goarSchema.Tag{},
	)
	if err != nil {
		fmt.Println("Failed to spawn: ", err)
		return
	}
	target2 := res.Id
	fmt.Println("spawn target2: ", target2)

	receive_step1(s, target1, target2, "receive2.lua")
	receive_step2(s, target1, target2)
}
