package main

import (
	"fmt"
	"time"

	"github.com/permadao/goar/schema"
	goarSchema "github.com/permadao/goar/schema"
)

const agentTestModule = "wNIB3qxEna1mifhY6uWiT-boEahHEZWr8pBEJaVu5I8"

func agentTestRT() error {
	res, err := s.SpawnAndWait(
		agentTestModule,
		scheduler,
		[]goarSchema.Tag{},
	)
	if err != nil {
		return fmt.Errorf("spawn agent test runtime: %w", err)
	}
	target := res.Id
	fmt.Println("spawn agent testrt: ", target)

	//sleep 1s
	time.Sleep(1 * time.Second)

	res, err = s.SendMessageAndWait(target, "",
		[]schema.Tag{
			{Name: "Action", Value: "Ping"},
			{Name: "Target", Value: target},
		})
	if err != nil {
		return fmt.Errorf("send ping failed: %w", err)
	}
	fmt.Println("ping ok, ", res)

	res, err = s.SendMessageAndWait(target, "",
		[]schema.Tag{
			{Name: "Action", Value: "Echo"},
			{Name: "Target", Value: target},
			{Name: "From", Value: "fallback-target"},
			{Name: "Data", Value: "from-params"},
		})
	if err != nil {
		return fmt.Errorf("send echo failed: %w", err)
	}
	fmt.Println("echo ok, ", res)

	return nil
}
