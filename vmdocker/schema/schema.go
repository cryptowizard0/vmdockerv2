package schema

import (
	vmmSchema "github.com/hymatrix/hymx/vmm/schema"
	goarSchema "github.com/permadao/goar/schema"
)

type SpawnRequest struct {
	Pid    string           `json:"pid"`
	Owner  string           `json:"owner"`
	CuAddr string           `json:"cu_addr"`
	Data   []byte           `json:"data"`
	Tags   []goarSchema.Tag `json:"tags"`
	Evn    vmmSchema.Env    `json:"env"`
}

type ApplyRequest struct {
	Meta   vmmSchema.Meta    `json:"meta"`
	From   string            `json:"from"`
	Params map[string]string `json:"params"`
}

type OutboxResponse struct {
	Result string `json:"result"`
	Status string `json:"status"`
}

type RuntimeCheckpointResponse struct {
	Status string `json:"status"`
	State  string `json:"state"`
}

type RuntimeRestoreRequest struct {
	Env   vmmSchema.Env    `json:"env"`
	Tags  []goarSchema.Tag `json:"tags"`
	State string           `json:"state"`
}

type RuntimeRestoreResponse struct {
	Status string `json:"status"`
}
