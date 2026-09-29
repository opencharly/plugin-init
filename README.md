# plugin-init

The `init` plugin KIND for [opencharly/charly](https://github.com/opencharly/charly) —
the init-system vocabulary: supervisord/systemd fragment assembly, the entrypoint,
and the service-management templates.

A KIND provider dispatches via the `pb Invoke(OpLoad)` envelope (it decodes the
authored `init:` entity into `spec.Init`), so it serves itself in BOTH placements
(compiled-in OR out-of-process) like the `matching`/`exampleprobe` verbs. The
embedded build vocabulary's `init:` entities (supervisord/systemd) flow through
this same path at box build.

## What it provides

| Capability | Surface |
|---|---|
| `kind:init` | the `init:` build-vocabulary entity — an init system's fragment-assembly + entrypoint + service-management templates |

## The kind

An authored `init:` node names an init system and its `service_schema` — the
`service_template` that renders a `service:` entry into a supervisord
`[program:NAME]` INI fragment or a systemd `[Unit]`/`[Service]`/`[Install]`
block. A `service:` entry is what SELECTS an init system: charly adds that init's
own candy to the composition automatically and target-aware (`supervisord` for a
container image, nothing extra for a machine venue that already has systemd).

```yaml
systemd:
  init:
    service_schema:
      service_template: |
        [Unit]
        Description={{.Description}}
        ...
```

## How to use it

The default `init:` vocabulary is embedded in the charly binary; a project
declares `init:` (inline or an imported vocab file) only to extend or override
it. The candy is compiled into charly — no candy composition is needed.

## Layout

- `candy/plugin-init/` — the plugin module: `plugin.go`, `resolve.go`,
  `schema/init.cue`, `params/cue_types_gen.go`, `cmd/serve/main.go`.
- `candy/plugin-init/charly.yml` — the `plugin-init:` candy entity.
- `charly.yml` — the root project manifest (`discover: candy`).
- `.github/workflows/tag-on-merge.yml` — CalVer tag + `CHANGELOG/` on merge.

## Related

- Owning skill: `/charly-image:image` — the `charly box` command family and the
  build vocabulary (`init:`/`distro:`/`builder:`/`resource:`). This candy carries
  no `skill:` entity of its own; the gap is tracked in
  [opencharly/opencharly#291](https://github.com/opencharly/opencharly/issues/291).
- `/charly-infrastructure:supervisord` — the supervisord `service_schema`
  template reference.
- `/charly-internals:plugin` — the plugin/provider model, including the `kind`
  provider class.
- [`opencharly/charly`](https://github.com/opencharly/charly) — the charly CLI.
