# AGENTS.md — plugin-init

Standalone plugin repo for the `init` plugin KIND (`kind:init`) — the
init-system build vocabulary (supervisord/systemd fragment assembly + entrypoint
+ service-management templates). The plugin is a Go module at
`candy/plugin-init/` (module path
`github.com/opencharly/plugin-init/candy/plugin-init`); the root `charly.yml`
only declares `discover: candy` so the repo is a project and its candy is
scanned.

Canonical files:

- `candy/plugin-init/charly.yml` — the `plugin-init:` candy entity (`plugin:`
  block, `plan:` check).
- `candy/plugin-init/` — the Go source: `plugin.go`, `resolve.go`,
  `schema/init.cue`, `params/cue_types_gen.go`, `cmd/serve/main.go`.
- `.github/workflows/tag-on-merge.yml` — CalVer tag + `CHANGELOG/` on merge.
- `README.md` — user overview only; never agent guidance.

## Load these skills first (R0)

- `/charly-internals:plugin` — the plugin authoring reference: the `plugin:`
  block, the unified Provider model, the per-plugin CUE-schema contract, the
  `kind` provider class. Load before touching the provider or schema.
- `/charly-image:image` — the build vocabulary the `init:` entity belongs to.
- `/charly-infrastructure:supervisord` — the supervisord `service_schema`
  template the kind renders.
- `/charly-internals:git-workflow` — before any git/PR action.

## Build / validate / test

- `go build ./...` in `candy/plugin-init/` — compile the plugin module.
- `go test ./...` in `candy/plugin-init/` — the plugin's render tests. NOTE:
  `render_service_hooks_test.go` currently FAILS on `origin/main` — it reads the
  shipped systemd template from a hardcoded `../../charly/charly.yml` that no
  longer resolves after the candy de-submodule cutover. Tracked in
  [opencharly/plugin-init#7](https://github.com/opencharly/plugin-init/issues/7).
- `charly box validate` at the repo root — the structural check (the candy +
  `plugin:` block, CUE schema).
- The merge gate is the **org-wide** `charly/pr-validator` (required check
  `validate / validate`, defined in `opencharly/.github`); this repo has **no**
  per-repo candy gate.
- The changed path is exercised at every box build that renders an init system
  (e.g. `check-pod` supervisord).

## Modify this repo

- Edit the `plugin-init:` candy entity, the Go source, and `schema/init.cue`
  **together** — the schema is the single source for the kind's `params/`
  struct.
- The package is `initkind` (not `init`) — `init` is a reserved Go identifier;
  the directory + kind keyword stay `init`.

## Landing

- The authoritative rulebook is the umbrella `AGENTS.md` in
  `opencharly/opencharly` and `charly/AGENTS.md` in the charly repo. Load
  `/charly-internals:git-workflow` before any git/PR action; history lives in
  `CHANGELOG/`. Do not restate its rules here.
