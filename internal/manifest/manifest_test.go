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
		t.Fatal("instance looper introuvable")
	}
	// Défauts alignés sur ceux de sooperlooper(1).
	if app.OSCPort != 9951 || app.Loops != 1 || app.Channels != 2 || app.LoopTime != 40 {
		t.Errorf("défauts sooperlooper incorrects: %+v", app)
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
	// name omis → <session>-<kind>.
	if got := strings.Join(m.AppNames(), " "); got != "bossa-carla bossa-hydrogen" {
		t.Errorf("AppNames() = %q", got)
	}
	if _, ok := m.FindApp("bossa-carla"); !ok {
		t.Error("bossa-carla introuvable")
	}
}

func TestResolveApp(t *testing.T) {
	m, err := load(t, valid)
	if err != nil {
		t.Fatal(err)
	}
	// Raccourci par kind : une seule instance carla.
	if a, err := m.ResolveApp("carla"); err != nil || a.Name != "clean" {
		t.Errorf("ResolveApp(carla) = %v, %v", a, err)
	}
	// Nom exact.
	if a, err := m.ResolveApp("looper"); err != nil || a.Kind != KindSooperLooper {
		t.Errorf("ResolveApp(looper) = %v, %v", a, err)
	}
	// Nom de session → erreur explicite.
	if _, err := m.ResolveApp("bossa"); err == nil || !strings.Contains(err.Error(), "session") {
		t.Errorf("ResolveApp(bossa) err = %v", err)
	}
	// Inconnu.
	if _, err := m.ResolveApp("zorglub"); err == nil {
		t.Error("ResolveApp(zorglub) devrait échouer")
	}
}

func TestValidateErrors(t *testing.T) {
	cases := map[string]struct {
		yaml    string
		wantErr string
	}{
		"kind inconnu": {
			yaml: `
sessions:
  s:
    apps:
      - {kind: ardour, name: x}
`,
			wantErr: "kind \"ardour\" inconnu",
		},
		"preset carla manquant": {
			yaml: `
sessions:
  s:
    apps:
      - {kind: carla, name: x}
`,
			wantErr: "'preset' requis",
		},
		"song hydrogen manquant": {
			yaml: `
sessions:
  s:
    apps:
      - {kind: hydrogen, name: x}
`,
			wantErr: "'song' requis",
		},
		"instance dupliquée entre sessions": {
			yaml: `
sessions:
  a:
    apps:
      - {kind: carla, name: x, preset: p.carla}
  b:
    apps:
      - {kind: carla, name: x, preset: q.carla}
`,
			wantErr: "uniques",
		},
		"noms identiques entre kinds différents": {
			yaml: `
sessions:
  s:
    apps:
      - {kind: carla, name: x, preset: p.carla}
      - {kind: hydrogen, name: x, song: s.h2song}
`,
			wantErr: "uniques",
		},
		"deux carla sans name (noms défautés collisionnent)": {
			yaml: `
sessions:
  s:
    apps:
      - {kind: carla, preset: p.carla}
      - {kind: carla, preset: q.carla}
`,
			wantErr: "uniques",
		},
		"port OSC dupliqué": {
			yaml: `
sessions:
  s:
    apps:
      - {kind: sooperlooper, name: a, osc_port: 9000}
      - {kind: sooperlooper, name: b, osc_port: 9000}
`,
			wantErr: "port OSC 9000",
		},
		"nom d'instance invalide": {
			yaml: `
sessions:
  s:
    apps:
      - {kind: carla, name: "a b", preset: p.carla}
`,
			wantErr: "nom invalide",
		},
		"patchbay sans fichier": {
			yaml: `
sessions:
  s:
    apps:
      - {kind: carla, name: x, preset: p.carla}
    patchbay:
      exclusive: true
`,
			wantErr: "patchbay sans champ 'file'",
		},
		"manifeste vide": {
			yaml:    `sessions: {}`,
			wantErr: "aucune session",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := load(t, tc.yaml)
			if err == nil {
				t.Fatalf("erreur attendue contenant %q, obtenu nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("erreur %q ne contient pas %q", err, tc.wantErr)
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
		t.Errorf("chemin absolu modifié: %q", got)
	}
	if got := m.Resolve("presets/p.carla"); got != filepath.Join(m.Dir, "presets/p.carla") {
		t.Errorf("chemin relatif mal résolu: %q", got)
	}
}
