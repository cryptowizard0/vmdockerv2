package vmdocker_test

// import (
// 	"context"
// 	"fmt"
// 	"testing"
// 	"time"

// 	nodeSchema "github.com/hymatrix/hymx/node/schema"
// 	vmmSchema "github.com/hymatrix/hymx/vmm/schema"
// 	vmdocker "github.com/cryptowizard0/vmdockerv2/vmdocker"
// 	goarSchema "github.com/permadao/goar/schema"
// 	"github.com/stretchr/testify/assert"
// )

// // go test -v ./vmm/vm_docker -run Test_VmDocker_Chekcpoint
// func Test_VmDocker_Chekcpoint(t *testing.T) {
// 	// ao env
// 	var (
// 		pid    = "vmdocker_checkpoint_test"
// 		owner  = "0x123"
// 		cuAddr = "0x456"
// 		tags   = []goarSchema.Tag{
// 			{
// 				Name:  "Module-Format",
// 				Value: vmmSchema.ModuleFormatGolua,
// 			},
// 		}
// 		env = vmmSchema.Env{
// 			Id:      pid,
// 			Owner:   owner,
// 			Process: nodeSchema.Process{},
// 			Module: nodeSchema.Module{
// 				Base: nodeSchema.Base{
// 					DataProtocol: nodeSchema.DataProtocol,
// 					Variant:      nodeSchema.Variant,
// 					Type:         nodeSchema.TypeModule,
// 				},
// 				ModuleFormat: "golua",
// 				MemoryLimit:  "500-mb",
// 				ComputeLimit: "9000000000000",
// 				Tags: []goarSchema.Tag{
// 					{
// 						Name:  "Content-Type",
// 						Value: "text/plain",
// 					},
// 				},
// 			},
// 			Nonce:       0,
// 			Sequence:    0,
// 			ReceivedSeq: map[string]int64{},
// 		}
// 	)

// 	// create & spawn
// 	vm, ierr := vmdocker.New(env, cuAddr, tags)
// 	if ierr != nil {
// 		t.Fatalf("create vm failed: %v", ierr)
// 	}

// 	dm, err := vmdocker.GetDockerManager()
// 	assert.NoError(t, err)
// 	assert.NotNil(t, dm)
// 	defer dm.RemoveContainer(context.Background(), pid)

// 	// wait for container start
// 	time.Sleep(5 * time.Second)

// 	// apply
// 	// eval: Name = "Tom"
// 	code := `
// 		print('Hello from lua!')
// 		Name = "Tom"
// 	`
// 	evalParams := map[string]string{
// 		"Action":       "Eval",
// 		"From":         owner,
// 		"Id":           "0x131313",
// 		"Owner":        owner,
// 		"Target":       pid,
// 		"Module":       "0x84534",
// 		"Block-Height": "100000",
// 		"Data":         code,
// 	}
// 	_, err = vm.Apply("", owner, "Eval", 1, evalParams)
// 	if err != nil {
// 		t.Fatalf("apply failed: %v", err)
// 	}

// 	// get Name should be "Tom"
// 	code = `
// 	Name
// 	`
// 	evalParams = map[string]string{
// 		"Action":       "Eval",
// 		"From":         owner,
// 		"Id":           "0x131313",
// 		"Owner":        owner,
// 		"Target":       pid,
// 		"Module":       "0x84534",
// 		"Block-Height": "100000",
// 		"Data":         code,
// 	}
// 	outbox, err := vm.Apply("", owner, "Eval", 1, evalParams)
// 	if err != nil {
// 		t.Fatalf("apply failed: %v", err)
// 	}
// 	output, ok := outbox.Output.(map[string]interface{})
// 	if !ok {
// 		t.Fatalf("output type assertion failed")
// 	}
// 	ret, ok := output["data"].(string)
// 	if !ok {
// 		t.Fatalf("data type assertion failed")
// 	}
// 	t.Logf("Name value: %s", ret)
// 	assert.Equal(t, "Tom", ret)

// 	// create checkpoint
// 	_, err = vm.CheckPoint(0)
// 	if err != nil {
// 		t.Fatalf("checkpoint failed: %v", err)
// 	}

// 	// eval: Name = "Jack"
// 	code = `
// 		print('Hello from lua!')
// 		Name = "Jack"
// 	`
// 	evalParams = map[string]string{
// 		"Action":       "Eval",
// 		"From":         owner,
// 		"Id":           "0x131313",
// 		"Owner":        owner,
// 		"Target":       pid,
// 		"Module":       "0x84534",
// 		"Block-Height": "100000",
// 		"Data":         code,
// 	}
// 	_, err = vm.Apply("", owner, "Eval", 1, evalParams)
// 	if err != nil {
// 		t.Fatalf("apply failed: %v", err)
// 	}

// 	// get Name should be "Jack"
// 	code = `
// 	Name
// 	`
// 	evalParams = map[string]string{
// 		"Action":       "Eval",
// 		"From":         owner,
// 		"Id":           "0x131313",
// 		"Owner":        owner,
// 		"Target":       pid,
// 		"Module":       "0x84534",
// 		"Block-Height": "100000",
// 		"Data":         code,
// 	}
// 	outbox, err = vm.Apply("", owner, "Eval", 1, evalParams)
// 	if err != nil {
// 		t.Fatalf("apply failed: %v", err)
// 	}
// 	output, ok = outbox.Output.(map[string]interface{})
// 	if !ok {
// 		t.Fatalf("output type assertion failed")
// 	}
// 	ret, ok = output["data"].(string)
// 	if !ok {
// 		t.Fatalf("data type assertion failed")
// 	}
// 	t.Logf("Name value: %s", ret)
// 	assert.Equal(t, "Jack", ret)

// 	err = dm.RemoveContainer(context.Background(), pid)
// 	if err != nil {
// 		t.Fatalf("remove container failed: %v", err)
// 	}
// 	// spawn
// 	vm, ierr = vmdocker.New(env, cuAddr, tags)
// 	if ierr != nil {
// 		t.Fatalf("create vm failed: %v", ierr)
// 	}

// 	// restore checkpoint
// 	checkpointName := fmt.Sprintf("checkpoint-%s-%d", pid, 0)
// 	err = vm.Restore([]byte(checkpointName))
// 	if err != nil {
// 		t.Fatalf("restore failed: %v", err)
// 	}

// 	time.Sleep(5 * time.Second)
// 	// get Name should be "Tom"
// 	code = `
// 	Name
// 	`
// 	evalParams = map[string]string{
// 		"Action":       "Eval",
// 		"From":         owner,
// 		"Id":           "0x131313",
// 		"Owner":        owner,
// 		"Target":       pid,
// 		"Module":       "0x84534",
// 		"Block-Height": "100000",
// 		"Data":         code,
// 	}
// 	outbox, err = vm.Apply("", owner, "Eval", 1, evalParams)
// 	if err != nil {
// 		t.Fatalf("apply failed: %v", err)
// 	}
// 	output, ok = outbox.Output.(map[string]interface{})
// 	if !ok {
// 		t.Fatalf("output type assertion failed")
// 	}
// 	ret, ok = output["data"].(string)
// 	if !ok {
// 		t.Fatalf("data type assertion failed")
// 	}
// 	t.Logf("Name value: %s", ret)
// 	assert.Equal(t, "Tom", ret)
// }
