package initkind

// resolve.go — candy/plugin-init's OpResolve legs (the init de-type, Cutover F).
// The init-system KNOWLEDGE — how a service_template / drop-in renders into a
// systemd unit or supervisord fragment, plus the restart/stdout policy mappings —
// lives HERE now. The host builds the entry-derived ServiceRenderContext (home-
// expanded, branch decisions precomputed) and hands it + the opaque init body; this
// renders the unit. The host re-validates the returned body for egress.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/opencharly/spec/spec"
)

// renderServiceUnit renders one service into a RenderedService using the chosen
// init system's templates (the service-render leg).
func renderServiceUnit(in spec.ServiceRenderInput) (spec.ServiceRenderReply, error) {
	var initDef spec.Init
	if err := json.Unmarshal(in.Init, &initDef); err != nil {
		return spec.ServiceRenderReply{}, fmt.Errorf("init render: decode init: %w", err)
	}
	if initDef.ServiceSchema == nil {
		return spec.ServiceRenderReply{}, fmt.Errorf("init render: init system has no service_schema")
	}
	schema := initDef.ServiceSchema
	ctx := in.Ctx
	out := &spec.RenderedService{}

	// Packaged-unit branch: reuse a distro-shipped unit, render only a drop-in.
	if ctx.PackagedUnit != "" {
		if !schema.SupportsPackaged {
			return spec.ServiceRenderReply{}, fmt.Errorf("init system %q does not support use_packaged (entry %s)", initDef.ManagementTool, ctx.Name)
		}
		if ctx.RenderDropin {
			text, err := renderInitTemplate("service-dropin", schema.DropinTemplate, ctx)
			if err != nil {
				return spec.ServiceRenderReply{}, fmt.Errorf("rendering dropin for %s: %w", ctx.Name, err)
			}
			path, err := renderInitTemplate("dropin-path", schema.DropinPathTemplate, ctx)
			if err != nil {
				return spec.ServiceRenderReply{}, fmt.Errorf("rendering dropin path for %s: %w", ctx.Name, err)
			}
			out.DropinText = text
			out.DropinPath = strings.TrimSpace(path)
		}
		return spec.ServiceRenderReply{Rendered: out}, nil
	}

	// Custom unit branch.
	if schema.ServiceTemplate == "" {
		return spec.ServiceRenderReply{}, fmt.Errorf("init system %q has no service_template for custom entries", initDef.ManagementTool)
	}
	text, err := renderInitTemplate("service-unit", schema.ServiceTemplate, ctx)
	if err != nil {
		return spec.ServiceRenderReply{}, fmt.Errorf("rendering unit for %s: %w", ctx.Name, err)
	}
	path, err := renderInitTemplate("service-path", schema.UnitPathTemplate, ctx)
	if err != nil {
		return spec.ServiceRenderReply{}, fmt.Errorf("rendering unit path for %s: %w", ctx.Name, err)
	}
	out.UnitText = text
	out.UnitPath = strings.TrimSpace(path)
	return spec.ServiceRenderReply{Rendered: out}, nil
}

// resolveInitConfig projects an authored spec.Init into a ResolvedInit — the
// build/label/entrypoint value envelope the kernel consumes for legs 2–4. Raw is
// the opaque body threaded back to the service-render leg.
func resolveInitConfig(in spec.InitResolveInput) (spec.InitResolveReply, error) {
	var d spec.Init
	if err := json.Unmarshal(in.Init, &d); err != nil {
		return spec.InitResolveReply{}, fmt.Errorf("init resolve config: decode: %w", err)
	}
	return spec.InitResolveReply{Resolved: &spec.ResolvedInit{
		CandyFields:          d.CandyFields,
		CandyFiles:           d.CandyFiles,
		DependsCandy:         d.DependsCandy,
		RequiresCapability:   d.RequiresCapability,
		Model:                d.Model,
		HeaderFile:           d.HeaderFile,
		FragmentDir:          d.FragmentDir,
		RelayTemplate:        d.RelayTemplate,
		StageName:            d.StageName,
		StageHeaderCopy:      d.StageHeaderCopy,
		StageFragmentCopy:    d.StageFragmentCopy,
		AssemblyTemplate:     d.AssemblyTemplate,
		SystemEnableTemplate: d.SystemEnableTemplate,
		PostAssemblyTemplate: d.PostAssemblyTemplate,
		Entrypoint:           d.Entrypoint,
		FallbackEntrypoint:   d.FallbackEntrypoint,
		ManagementTool:       d.ManagementTool,
		ManagementCommands:   d.ManagementCommands,
		LabelKey:             d.LabelKey,
		ServiceSchema:        d.ServiceSchema,
		Raw:                  in.Init,
	}}, nil
}

func renderInitTemplate(name, tmpl string, data any) (string, error) {
	if tmpl == "" {
		return "", nil
	}
	t, err := template.New(name).Funcs(serviceRenderFuncs()).Parse(tmpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// directive is one rendered init directive: a template ranges over these and emits
// "{{.Key}}={{.Value}}" (or the init's own spelling) per element.
type directive struct {
	Key   string
	Value string
}

// serviceRenderFuncs are the init-system template funcs (the restart/stdout policy
// mappings each init system's service_template uses).
func serviceRenderFuncs() template.FuncMap {
	return template.FuncMap{
		"join": strings.Join,
		"derefBool": func(b *bool) bool {
			if b == nil {
				return false
			}
			return *b
		},
		"systemdRestart": func(r string) string {
			switch r {
			case "always":
				return "always"
			case "on-failure":
				return "on-failure"
			case "unless-stopped":
				return "always"
			case "no":
				return "no"
			}
			// Unset defaults to always — parity with supervisord's
			// autorestart=true default (init polymorphism must not change
			// restart behavior between substrates).
			return "always"
		},
		"supervisordRestart": func(r string) string {
			switch r {
			case "always":
				return "true"
			case "on-failure":
				return "unexpected"
			case "unless-stopped":
				return "true"
			case "no", "":
				return "false"
			}
			return "false"
		},
		// stdout: is the tri-state "journal" | "none" | "file:<path>". systemd has no
		// "none" - its sink for discard is "null" - so passing the vocabulary word
		// through unmapped emitted an invalid StandardOutput= that systemd rejects at
		// unit load. Masked until now because the systemd template never called this.
		// initDirectives flattens unit_options for ONE init into ordered Key/Value
		// pairs a template can range over.
		//
		// unit_options is keyed init-name -> directive -> value, and a value may be a
		// scalar OR a list: systemd repeats directives such as RuntimeDirectory= or
		// ReadWritePaths= once per element, so a list must expand to N lines rather
		// than render as one unusable Go slice literal. Templates cannot branch on a
		// dynamic type, which is why the flattening happens here.
		//
		// A template reads only its OWN init's key, so a directive meant for systemd
		// can never leak into an OpenRC script. Ordering is by directive name: unit
		// text must not reorder between builds.
		// waitForScript lowers wait_for into ONE POSIX sh command a template can place
		// wherever its init runs pre-start work.
		//
		// Returns "" when there is nothing to wait for, so a template's {{if}} omits the
		// whole branch rather than emitting an empty guard.
		//
		// Paths are embedded in DOUBLE quotes on purpose: a wait_for routinely names
		// "${XDG_RUNTIME_DIR}/wayland-1", and the shell must expand that at start time,
		// not at render time. The snippet itself contains no single quotes, so a caller
		// can wrap the whole thing in single quotes (systemd's ExecStartPre= does).
		//
		// On timeout it exits NON-ZERO with the paths named. That is the point of
		// lowering per-init rather than emitting a wrapper: the wait is supervised, so a
		// timeout fails the unit and shows up in its status, instead of a wrapper exiting
		// and looking like the daemon crashed.
		"waitForScript": func(w *spec.ServiceWaitFor) string {
			if w == nil || len(w.Paths) == 0 {
				return ""
			}
			secs := 30
			if t := strings.TrimSuffix(strings.TrimSpace(w.Timeout), "s"); t != "" {
				if n, err := strconv.Atoi(t); err == nil && n > 0 {
					secs = n
				}
			}
			tests := make([]string, 0, len(w.Paths))
			for _, path := range w.Paths {
				tests = append(tests, fmt.Sprintf(`[ -e "%s" ]`, path))
			}
			return fmt.Sprintf(
				`n=0; until %s; do n=$((n+1)); if [ "$n" -ge %d ]; then `+
					`echo "wait_for: timed out after %ds waiting for: %s" >&2; exit 1; fi; sleep 1; done`,
				strings.Join(tests, " && "), secs, secs, strings.Join(w.Paths, " "))
		},
		"initDirectives": func(opts map[string]map[string]any, init string) []directive {
			byName := opts[init]
			if len(byName) == 0 {
				return nil
			}
			names := make([]string, 0, len(byName))
			for k := range byName {
				names = append(names, k)
			}
			sort.Strings(names)
			out := []directive{}
			for _, n := range names {
				switch v := byName[n].(type) {
				case string:
					out = append(out, directive{Key: n, Value: v})
				case []string:
					for _, e := range v {
						out = append(out, directive{Key: n, Value: e})
					}
				case []any:
					// The YAML/JSON decode path yields []any, not []string.
					for _, e := range v {
						out = append(out, directive{Key: n, Value: fmt.Sprint(e)})
					}
				default:
					// bool/int and anything else: render its natural form rather than
					// dropping a directive the author explicitly asked for.
					out = append(out, directive{Key: n, Value: fmt.Sprint(v)})
				}
			}
			return out
		},
		"systemdStdout": func(s string) string {
			if after, ok := strings.CutPrefix(s, "file:"); ok {
				return "append:" + after
			}
			switch s {
			case "none":
				return "null"
			case "journal", "":
				return "journal"
			}
			return s
		},
		// openrcLog maps the same tri-state to OpenRC's output_log/error_log, which
		// take a PATH. "journal" has no OpenRC analogue - the default is to inherit
		// the supervisor's own descriptors - so it renders empty and the template
		// omits the directive entirely rather than inventing a sink.
		"openrcLog": func(s string) string {
			if after, ok := strings.CutPrefix(s, "file:"); ok {
				return after
			}
			if s == "none" {
				return "/dev/null"
			}
			return ""
		},
		"supervisordLog": func(s string) string {
			if after, ok := strings.CutPrefix(s, "file:"); ok {
				return after
			}
			switch s {
			case "none":
				return "/dev/null"
			case "journal", "":
				return "/dev/fd/1"
			}
			return s
		},
		"supervisordLogMaxbytes": func(s string) string {
			if strings.HasPrefix(s, "file:") {
				return "10MB"
			}
			return "0"
		},
	}
}
