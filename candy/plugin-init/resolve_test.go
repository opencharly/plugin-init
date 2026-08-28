package initkind

import (
	"strings"
	"testing"

	"github.com/opencharly/spec/spec"
)

// TestRestartMappingFuncs guards the abstract restart: → init-system policy mappings.
func TestRestartMappingFuncs(t *testing.T) {
	funcs := serviceRenderFuncs()

	systemdRestart := funcs["systemdRestart"].(func(string) string)
	if got := systemdRestart("always"); got != "always" {
		t.Errorf("systemdRestart(always) = %q", got)
	}
	if got := systemdRestart("on-failure"); got != "on-failure" {
		t.Errorf("systemdRestart(on-failure) = %q", got)
	}
	if got := systemdRestart("unless-stopped"); got != "always" {
		t.Errorf("systemdRestart(unless-stopped) = %q (want always)", got)
	}
	// Unset defaults to always — init-polymorphism parity with supervisord's
	// autorestart=true default (a service that self-heals in a pod must not
	// stay dead in a VM).
	if got := systemdRestart(""); got != "always" {
		t.Errorf("systemdRestart(empty) = %q (want always)", got)
	}

	supRestart := funcs["supervisordRestart"].(func(string) string)
	if got := supRestart("always"); got != "true" {
		t.Errorf("supervisordRestart(always) = %q", got)
	}
	if got := supRestart("on-failure"); got != "unexpected" {
		t.Errorf("supervisordRestart(on-failure) = %q", got)
	}
	if got := supRestart("no"); got != "false" {
		t.Errorf("supervisordRestart(no) = %q", got)
	}
}

// TestSupervisordStdoutLogging guards the supervisord stdout_logfile mapping:
// unset keeps the historical /dev/fd/1 default, file:<path> yields a rotating
// dedicated log, none is /dev/null.
func TestSupervisordStdoutLogging(t *testing.T) {
	fns := serviceRenderFuncs()
	logf := fns["supervisordLog"].(func(string) string)
	maxb := fns["supervisordLogMaxbytes"].(func(string) string)
	cases := []struct{ in, wantLog, wantMax string }{
		{"", "/dev/fd/1", "0"},
		{"journal", "/dev/fd/1", "0"},
		{"none", "/dev/null", "0"},
		{"file:/home/user/.local/share/selkies/selkies.log", "/home/user/.local/share/selkies/selkies.log", "10MB"},
	}
	for _, c := range cases {
		if got := logf(c.in); got != c.wantLog {
			t.Errorf("supervisordLog(%q) = %q, want %q", c.in, got, c.wantLog)
		}
		if got := maxb(c.in); got != c.wantMax {
			t.Errorf("supervisordLogMaxbytes(%q) = %q, want %q", c.in, got, c.wantMax)
		}
	}
}

// TestSystemdStdoutLogging mirrors TestSupervisordStdoutLogging for systemd. The
// case that matters is "none": systemd has no such value — its discard sink is
// "null" — so passing the vocabulary word through unmapped emitted a
// StandardOutput= systemd rejects at unit load.
func TestSystemdStdoutLogging(t *testing.T) {
	logf := serviceRenderFuncs()["systemdStdout"].(func(string) string)
	valid := map[string]bool{
		"inherit": true, "null": true, "tty": true, "journal": true,
		"kmsg": true, "journal+console": true, "socket": true,
	}
	cases := []struct{ in, want string }{
		{"", "journal"},
		{"journal", "journal"},
		{"none", "null"},
		{"file:/var/log/svc.log", "append:/var/log/svc.log"},
	}
	for _, c := range cases {
		got := logf(c.in)
		if got != c.want {
			t.Errorf("systemdStdout(%q) = %q, want %q", c.in, got, c.want)
		}
		// Every rendering must be something systemd actually accepts.
		if !valid[got] && !strings.HasPrefix(got, "append:") &&
			!strings.HasPrefix(got, "file:") && !strings.HasPrefix(got, "truncate:") &&
			!strings.HasPrefix(got, "fd:") {
			t.Errorf("systemdStdout(%q) = %q, which is not a valid StandardOutput= value", c.in, got)
		}
	}
}

// TestOpenrcLogging: OpenRC's output_log/error_log take a PATH, and "journal" has
// no OpenRC analogue, so it must render empty for the template to omit the
// directive rather than invent a sink.
func TestOpenrcLogging(t *testing.T) {
	logf := serviceRenderFuncs()["openrcLog"].(func(string) string)
	cases := []struct{ in, want string }{
		{"", ""},
		{"journal", ""},
		{"none", "/dev/null"},
		{"file:/var/log/svc.log", "/var/log/svc.log"},
	}
	for _, c := range cases {
		if got := logf(c.in); got != c.want {
			t.Errorf("openrcLog(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// initDirectives flattens unit_options for ONE init into ordered pairs. The cases that
// matter: a list must expand to N directives (systemd repeats RuntimeDirectory= and
// ReadWritePaths= once per element, so collapsing a list would emit one unusable Go
// slice literal), []any must behave like []string because that is what the YAML/JSON
// decode path actually produces, and one init must never see another's directives.
func TestInitDirectivesFlattensUnitOptions(t *testing.T) {
	fn := serviceRenderFuncs()["initDirectives"].(func(map[string]map[string]any, string) []directive)

	opts := map[string]map[string]any{
		"systemd": {
			"KillMode":         "process",
			"RuntimeDirectory": []string{"cstream", "cstream/leaders"},
			"ReadWritePaths":   []any{"/run/a", "/run/b"},
			"TasksMax":         512,
		},
		"openrc": {"supervise_daemon_args": "--foo"},
	}

	got := fn(opts, "systemd")
	want := []directive{
		{"KillMode", "process"},
		{"ReadWritePaths", "/run/a"},
		{"ReadWritePaths", "/run/b"},
		{"RuntimeDirectory", "cstream"},
		{"RuntimeDirectory", "cstream/leaders"},
		{"TasksMax", "512"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d directives %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			// Ordering is asserted too: unit text must not reorder between builds.
			t.Errorf("directive %d = %v, want %v", i, got[i], want[i])
		}
	}

	if oc := fn(opts, "openrc"); len(oc) != 1 || oc[0].Key != "supervise_daemon_args" {
		t.Errorf("openrc got %v — an init must see only its OWN directives", oc)
	}
	if none := fn(opts, "supervisord"); len(none) != 0 {
		t.Errorf("supervisord got %v, want nothing", none)
	}
	if none := fn(nil, "systemd"); len(none) != 0 {
		t.Errorf("a nil map produced %v, want nothing", none)
	}
}

// waitForScript lowers wait_for into one POSIX sh command. The properties that
// matter are the ones that make it SAFE to embed and correct to run.
func TestWaitForScript(t *testing.T) {
	fn := serviceRenderFuncs()["waitForScript"].(func(*spec.ServiceWaitFor) string)

	if got := fn(nil); got != "" {
		t.Errorf("nil wait_for produced %q, want empty so the template omits the branch", got)
	}
	if got := fn(&spec.ServiceWaitFor{}); got != "" {
		t.Errorf("empty paths produced %q, want empty", got)
	}

	got := fn(&spec.ServiceWaitFor{
		Paths:   []string{"${XDG_RUNTIME_DIR}/wayland-1", "/run/broker.sock"},
		Timeout: "45s",
	})

	// Embeddable: systemd wraps this in single quotes, so the snippet must contain
	// none of its own.
	if strings.Contains(got, "'") {
		t.Errorf("script contains a single quote and cannot be wrapped by ExecStartPre=: %s", got)
	}
	// Double quotes, not single: "${XDG_RUNTIME_DIR}/wayland-1" has to expand at
	// START time. Single-quoting it would wait forever on a literal path.
	if !strings.Contains(got, `[ -e "${XDG_RUNTIME_DIR}/wayland-1" ]`) {
		t.Errorf("path is not embedded in double quotes, so the shell will not expand it: %s", got)
	}
	// Every path is a precondition, not just the first.
	if !strings.Contains(got, `[ -e "/run/broker.sock" ]`) || !strings.Contains(got, "&&") {
		t.Errorf("not all paths are required: %s", got)
	}
	// The declared timeout is honoured, not a default. This is the labwc bug in
	// assertion form: that wrapper declared 30 and waited 15.
	if !strings.Contains(got, "-ge 45") {
		t.Errorf("declared timeout 45s not honoured: %s", got)
	}
	// Failure must be loud and non-zero: a silent give-up would start the service
	// against a socket that never appeared.
	if !strings.Contains(got, "exit 1") || !strings.Contains(got, "timed out") {
		t.Errorf("timeout does not fail loudly: %s", got)
	}
	// The message names what it waited for; "timed out" alone is the diagnostic
	// dead-end this project keeps paying for.
	if !strings.Contains(got, "/run/broker.sock") {
		t.Errorf("the timeout message does not name the paths: %s", got)
	}

	// An absent timeout falls back to a default rather than waiting forever.
	def := fn(&spec.ServiceWaitFor{Paths: []string{"/a"}})
	if !strings.Contains(def, "-ge 30") {
		t.Errorf("no default timeout: %s", def)
	}
	// A malformed timeout must not silently become zero, which would fail instantly.
	bad := fn(&spec.ServiceWaitFor{Paths: []string{"/a"}, Timeout: "soon"})
	if !strings.Contains(bad, "-ge 30") {
		t.Errorf("a malformed timeout did not fall back to the default: %s", bad)
	}
}
