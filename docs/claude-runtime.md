# Claude Runtime In VMDocker

This document only describes Claude-related usage from the `vmdocker` side.

## 1. `.env` Configuration

For local development, the example file is:

- `vmdocker/examples/.env_example`

The example loader reads:

1. `.env`
2. `examples/.env`
3. `../.env`

If the same variable already exists in the shell, the file value does not override it.

### Recommended Example

```dotenv
VMDOCKER_URL=http://127.0.0.1:8080
VMDOCKER_PRIVATE_KEY=<your-wallet-private-key>
VMDOCKER_SCHEDULER=<scheduler-0x-address>
VMDOCKER_MODULE_ID=<your-claude-module-id>

ANTHROPIC_API_KEY=<your-anthropic-api-key>
ANTHROPIC_BASE_URL=
ANTHROPIC_MODEL=claude-sonnet-4-5
CLAUDE_MODEL=
CLAUDE_CODE_FLAGS=

RUNTIME_BACKEND=sandbox

CLAUDE_CHAT_COMMAND=Hello from VMDocker
CLAUDE_SPAWN_WAIT_TIMEOUT=10m
CLAUDE_MESSAGE_WAIT_TIMEOUT=5m
```

### Env Reference

| Key | Required | Description |
| --- | --- | --- |
| `VMDOCKER_URL` | yes | Hymx/VMDocker node URL |
| `VMDOCKER_PRIVATE_KEY` | yes | signer key used by the examples |
| `VMDOCKER_MODULE_ID` | yes | Claude-capable module id |
| `VMDOCKER_SCHEDULER` | yes | scheduler address passed to `s.Spawn(...)` |
| `ANTHROPIC_API_KEY` | yes | forwarded to runtime as `Container-Env-ANTHROPIC_API_KEY` |
| `ANTHROPIC_BASE_URL` | no | forwarded to runtime as `Container-Env-ANTHROPIC_BASE_URL` |
| `ANTHROPIC_MODEL` | no | forwarded to runtime as `Container-Env-ANTHROPIC_MODEL` |
| `CLAUDE_MODEL` | no | local fallback when `ANTHROPIC_MODEL` is unset |
| `CLAUDE_CODE_FLAGS` | no | forwarded to runtime as `Container-Env-CLAUDE_CODE_FLAGS` |
| `RUNTIME_BACKEND` | no | `docker` or `sandbox`; becomes `Runtime-Backend` |
| `CLAUDE_CHAT_COMMAND` | no | default message for `claude_chat` example |
| `CLAUDE_SPAWN_WAIT_TIMEOUT` | no | spawn wait timeout in examples |
| `CLAUDE_MESSAGE_WAIT_TIMEOUT` | no | apply wait timeout in examples |

### Notes

- `ANTHROPIC_MODEL` is the actual runtime env forwarded into the Claude container.
- `CLAUDE_MODEL` is only a local fallback used while building Claude spawn tags.
- `RUNTIME_BACKEND` controls how `vmdocker` launches the runtime instance.

## 2. Spawn

`examples/claude.go` uses:

```go
func spawnClaude() string {
	// ...
	s.Spawn(module, scheduler, buildClaudeSpawnTags(...))
  // ... 
}
```

The Claude spawn tags are:

| Tag Name | Required | Source | Meaning |
| --- | --- | --- | --- |
| `Container-Env-RUNTIME_TYPE` | yes | fixed | must be `claude` |
| `Container-Env-ANTHROPIC_API_KEY` | yes | `ANTHROPIC_API_KEY` | Claude API key |
| `Container-Env-ANTHROPIC_BASE_URL` | no | `ANTHROPIC_BASE_URL` | Anthropic-compatible proxy URL |
| `Container-Env-ANTHROPIC_MODEL` | no | `ANTHROPIC_MODEL` or `CLAUDE_MODEL` | model selection |
| `Container-Env-CLAUDE_CODE_FLAGS` | no | `CLAUDE_CODE_FLAGS` | extra Claude CLI flags |
| `Runtime-Backend` | no | `RUNTIME_BACKEND` | `docker` or `sandbox` |

`vmdocker` converts every `Container-Env-*` tag into a real container environment variable.

Example:

```text
Container-Env-ANTHROPIC_API_KEY=xxx
```

becomes:

```text
ANTHROPIC_API_KEY=xxx
```

inside the runtime container.

### Spawn Parameters

| Input | Required | Description |
| --- | --- | --- |
| module id | yes | Claude-capable module generated from `vmdocker_agent` |
| scheduler | yes | scheduler passed into `s.Spawn(...)` |
| Claude env tags | yes | runtime type and Anthropic config |
| runtime backend tag | no | explicit backend selection |

### Spawn Return

At runtime HTTP level, `/vmm/spawn` returns:

```json
{"status":"ok"}
```

At `vmdocker` example level, the useful result is:

- the spawn request succeeds
- a new process id is produced after waiting for the response

### Spawn Example

```bash
cd /Users/webbergao/work/src/HymxWorkspace/vmdocker
go run ./examples claude_spawn
```

## 3. Apply

`vmdocker` forwards apply requests to the runtime as:

```json
{
  "from": "<message-sender>",
  "meta": {
    "Action": "...",
    "Sequence": 1,
    "Params": {...},
    "Data": "..."
  },
  "params": {
    "...": "..."
  }
}
```

Documented Claude actions:

- `Execute`
- `Chat`

Accepted prompt aliases for both actions:

- `command`
- `Command`
- `prompt`
- `Prompt`
- `input`
- `Input`
- `data`
- `Data`
- `Meta.Data`

### Action 1: `Execute`

`Execute` is not a direct shell-command execution API.

In the current Claude runtime, `Execute` is implemented by passing the provided text to the Claude CLI as:

```bash
claude -p "<your prompt>" --output-format json --dangerously-skip-permissions ...
```

That means:

- the runtime itself does not run `Command` as `/bin/sh -c <Command>`
- the `Command` field is used as prompt text
- `Execute` is handled as a Claude prompt request, not as a raw shell wrapper
- the main difference is that the result tags keep `Action=Execute`

Claude may still decide to use its own tools during the prompt run, including file operations or command execution inside the workspace, because the runtime invokes Claude with `--dangerously-skip-permissions`. But that behavior comes from Claude's tool use inside the prompt session, not from `vmdocker` directly treating `Execute` as a shell wrapper.

#### Parameters

| Field | Required | Description |
| --- | --- | --- |
| `Action=Execute` | recommended | explicit action |
| `Command` or equivalent prompt field | yes | prompt text passed to `claude -p` |

#### Example

```go
resp, err := s.SendMessage(target, "", []schema.Tag{
  {Name: "Action", Value: "Execute"},
  {Name: "Command", Value: "Inspect the repository and describe the top-level directories."},
})
```

#### Return

| Field | Type | Meaning |
| --- | --- | --- |
| `Data` | string | Claude reply text |
| `Output` | string | same reply text |
| `Messages[0].Target` | string | reply target |
| `Messages[0].Tags` | array | includes `Action=Execute` |

So the current semantic model is:

- `Execute`: prompt -> Claude reply, labeled as execution-style intent
- `Chat`: prompt -> Claude reply, with chat-shaped `Output`

Typical tags:

- `Runtime=claude`
- `SessionID=<session-id>`
- `Reference=<reference>`
- `Reply=<reply-text>`
- `Action=Execute`

### Action 2: `Chat`

Use `Chat` when the caller expects a chat-style output payload.

#### Parameters

| Field | Required | Description |
| --- | --- | --- |
| `Action=Chat` | recommended | explicit action |
| `Command` or equivalent prompt field | yes | chat prompt |

#### Example

```go
resp, err := s.SendMessage(target, "", []schema.Tag{
  {Name: "Action", Value: "Chat"},
  {Name: "Command", Value: "Reply in one sentence: what runtime are you?"},
})
```

#### Return

| Field | Type | Meaning |
| --- | --- | --- |
| `Data` | string | Claude reply text |
| `Output.action` | string | always `Chat` |
| `Output.reply` | string | Claude reply text |
| `Messages[0].Tags` | array | includes `Action=Chat` |

Typical `Output`:

```json
{
  "action": "Chat",
  "reply": "Hello, I am the Claude runtime running behind VMDocker."
}
```
