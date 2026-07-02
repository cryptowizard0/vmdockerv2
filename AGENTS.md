# AGENTS.md

This file defines repo-local agent guidance for `vmdockerv2`.

## Base Rules

Apply the shared workspace rules from:

- `/Users/webbergao/work/src/HymxWorkspace/AGENTS.md`

Also follow the shared Go coding standard from:

- `/Users/webbergao/work/src/HymxWorkspace/docs/golang-coding-standards.md`

## Runtime Workspace Contract

When working on runtime creation, sandbox permissions, checkpoint/restore, or debugging spawned process state, treat this document as required context:

- `/Users/webbergao/work/src/HymxWorkspace/vmdockerv2/spec/sandbox-workspace-layout.md`

That document defines:

- the `cmd/sandbox_workspace/<pid>` directory contract,
- expected ownership and non-root runtime behavior,
- read-only rootfs expectations,
- the intended writable scope inside runtime containers.

## Validation

For code changes in this repo, prefer:

```bash
cd /Users/webbergao/work/src/HymxWorkspace/vmdockerv2
go test ./...
go build -o ./build/hymx-node ./cmd
```
