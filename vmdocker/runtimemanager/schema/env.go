package schema

import "os"

var (
	DockerVersion = "1.47"
	ExprotPort    = "8080/tcp"
	AllowHost     = "127.0.0.1"             // Only host machine can access the runtime
	MaxMem        = 12 * 1024 * 1024 * 1024 // max 12GB memory
	CheckpointDir = "checkpoints"

	// use mount to share models
	UseMount    = false
	MountSource = os.ExpandEnv("$HOME/.ollama/models")
	MountTarget = "/app/models"
)
