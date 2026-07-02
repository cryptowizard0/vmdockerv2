package main

import (
	"fmt"

	"github.com/permadao/goar/schema"
	goarSchema "github.com/permadao/goar/schema"
)

func eval() {
	res, err := s.SpawnAndWait(
		module,
		scheduler,
		[]goarSchema.Tag{},
	)
	if err != nil {
		fmt.Println("Failed to spawn: ", err)
		return
	}

	target := res.Id
	fmt.Println("spawn ok, pid: ", res.Id)

	code := `
		print('Hello from lua!')
		Name = 'Hello'
		Cache({Name = 'World'})
		Cache({Name2 = 'World2'})
	`

	res, err = s.SendMessageAndWait(target, code,
		[]schema.Tag{
			{Name: "Action", Value: "Eval"},
			{Name: "Target", Value: target},
			{Name: "Data", Value: code},
		})
	if err != nil {
		fmt.Println("handler error: ", err)
		return
	}
	fmt.Println("res, ", res)
}
