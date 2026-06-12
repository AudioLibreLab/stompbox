package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func load(t *testing.T, yaml string) (*Manifest, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stompbox.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	return Load(path)
}

const valid = `
sessions:
  bossa:
    apps:
      - kind: carla
        name: clean
        preset: presets/clean.carla
      - kind: hydrogen
        name: groove
        song: songs/bossa.h2song
      - kind: sooperlooper
        name: looper
    patchbay:
      file: patchbays/bossa.qpwgraph
`

func TestLoadValidAppliesDefaults(t *testing.T) {
	m, err := load(t, valid)
	if err != nil {
		t.Fatal(err)
	}
	app, ok := m.FindApp("looper")
	if !ok {
		t.Fatal("instance looper not found")
	}
	// Defaults aligned with sooperlooper(1).
	if app.OSCPort != 9951 || app.Loops != 1 || app.Channels != 2 || app.LoopTime != 40 {
		t.Errorf("incorrect sooperlooper defaults: %+v", app)
	}
	if got := app.Unit(); got != "sooperlooper@looper.service" {
		t.Errorf("Unit() = %q", got)
	}
	if got := strings.Join(m.AppNames(), " "); got != "clean groove looper" {
		t.Errorf("AppNames() = %q", got)
	}
}

func TestDefaultName(t *testing.T) {
	m, err := load(t, `
sessions:
  bossa:
    apps:
      - {kind: carla, preset: p.carla}
      - {kind: hydrogen, song: s.h2song}
`)
	if err != nil {
		t.Fatal(err)
	}
	// name omitted → <session>-<kind>.
	if got := strings.Join(m.AppNames(), " "); got != "bossa-carla bossa-hydrogen" {
		t.Errorf("AppNames() = %q", got)
	}
	if _, ok := m.FindApp("bossa-carla"); !ok {
		t.Error("bossa-carla not found")
	}
}

func TestResolveApp(t *testing.T) {
	m, err := load(t, valid)
	if err != nil {
		t.Fatal(err)
	}
	// Shortcut by kind: a single carla instance.
	if a, err := m.ResolveApp("carla"); err != nil || a.Name != "clean" {
		t.Errorf("ResolveApp(carla) = %v, %v", a, err)
	}
	// Exact name.
	if a, err := m.ResolveApp("looper"); err != nil || a.Kind != KindSooperLooper {
		t.Errorf("ResolveApp(looper) = %v, %v", a, err)
	}
	// Session name → explicit error.
	if _, err := m.ResolveApp("bossa"); err == nil || !strings.Contains(err.Error(), "session") {
		t.Errorf("ResolveApp(bossa) err = %v", err)
	}
	// Unknown.
	if _, err := m.ResolveApp("zorglub"); err == nil {
		t.Error("ResolveApp(zorglub) should fail")
	}
}

func TestValidateErrors(t *testing.T) {
	cases := map[string]struct {
		yaml    string
		wantErr string
	}{
		"unknown kind": {
			yaml: `
sessions:
  s:
    apps:
      - {kind: ardour, name: x}
`,
			wantErr: `unknown kind "ardour"`,
		},
		"missing carla preset": {
			yaml: `
sessions:
  s:
    apps:
      - {kind: carla, name: x}
`,
			wantErr: "'preset' field required",
		},
		"missing hydrogen song": {
			yaml: `
sessions:
  s:
    apps:
      - {kind: hydrogen, name: x}
`,
			wantErr: "'song' field required",
		},
		"duplicate instance across sessions": {
			yaml: `
sessions:
  a:
    apps:
      - {kind: carla, name: x, preset: p.carla}
  b:
    apps:
      - {kind: carla, name: x, preset: q.carla}
`,
			wantErr: "unique",
		},
		"same name across different kinds": {
			yaml: `
sessions:
  s:
    apps:
      - {kind: carla, name: x, preset: p.carla}
      - {kind: hydrogen, name: x, song: s.h2song}
`,
			wantErr: "unique",
		},
		"two carla without name (defaulted names collide)": {
			yaml: `
sessions:
  s:
    apps:
      - {kind: carla, preset: p.carla}
      - {kind: carla, preset: q.carla}
`,
			wantErr: "unique",
		},
		"duplicate OSC port": {
			yaml: `
sessions:
  s:
    apps:
      - {kind: sooperlooper, name: a, osc_port: 9000}
      - {kind: sooperlooper, name: b, osc_port: 9000}
`,
			wantErr: "OSC port 9000",
		},
		"invalid instance name": {
			yaml: `
sessions:
  s:
    apps:
      - {kind: carla, name: "a b", preset: p.carla}
`,
			wantErr: "invalid name",
		},
		"patchbay without file": {
			yaml: `
sessions:
  s:
    apps:
      - {kind: carla, name: x, preset: p.carla}
    patchbay:
      exclusive: true
`,
			wantErr: "missing 'file'",
		},
		"empty manifest": {
			yaml:    `sessions: {}`,
			wantErr: "no sessions",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := load(t, tc.yaml)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	m, err := load(t, valid)
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Resolve("/abs/p.carla"); got != "/abs/p.carla" {
		t.Errorf("absolute path modified: %q", got)
	}
	if got := m.Resolve("presets/p.carla"); got != filepath.Join(m.Dir, "presets/p.carla") {
		t.Errorf("relative path incorrectly resolved: %q", got)
	}
}
