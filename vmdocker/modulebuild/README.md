# modulebuild

Profile-driven module builder (spec §5-§8). `profile.toml` -> standardized
Dockerfile -> `docker build`/`docker save` -> container-tar module
(`image.tar.gz` + `profile.toml`), consumed by `cmd/module` which signs it via
the hymx SDK into `mod-<id>.json`.

Runtime Export/Import (adds `public.zip`) and spawn/load compatibility for the
new `hymx.vmdockerv2.v0.0.1` format are separate plans (P2/P3/P4).
Platform adapter binary + ENTRYPOINT wrapper are injected via `-agent-bin` /
`-wrapper` (B2); their canonical supply mechanism is a follow-up.
