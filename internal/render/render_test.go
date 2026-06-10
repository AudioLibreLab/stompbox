package render

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/AudioLibreLab/stompbox/internal/manifest"
)

var update = flag.Bool("update", false, "réécrit les fichiers golden")

// TestRenderGolden compare le rendu complet du manifeste de test aux
// fichiers de référence. Régénération : go test ./internal/render -update
func TestRenderGolden(t *testing.T) {
	m, err := manifest.Load("testdata/stompbox.yaml")
	if err != nil {
		t.Fatal(err)
	}
	// Répertoire fixe pour que les chemins absolus résolus soient
	// indépendants de l'emplacement du dépôt.
	m.Dir = "/studio"

	files, err := Render(m)
	if err != nil {
		t.Fatal(err)
	}

	if *update {
		if err := os.RemoveAll("testdata/golden"); err != nil {
			t.Fatal(err)
		}
	}

	seen := map[string]bool{}
	for _, f := range files {
		seen[f.Path] = true
		golden := filepath.Join("testdata/golden", f.Path)
		if *update {
			if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(golden, []byte(f.Content), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(golden)
		if err != nil {
			t.Errorf("%s: fichier golden manquant (lancer go test -update): %v", f.Path, err)
			continue
		}
		if f.Content != string(want) {
			t.Errorf("%s: rendu différent du golden\n--- attendu ---\n%s\n--- obtenu ---\n%s", f.Path, want, f.Content)
		}
	}

	// Détecte les goldens orphelins (artefact supprimé du rendu).
	if !*update {
		err := filepath.Walk("testdata/golden", func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return err
			}
			rel, _ := filepath.Rel("testdata/golden", path)
			if !seen[rel] {
				t.Errorf("golden orphelin: %s n'est plus généré", rel)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestCheckPathsWarnsOnMissingFiles(t *testing.T) {
	m, err := manifest.Load("testdata/stompbox.yaml")
	if err != nil {
		t.Fatal(err)
	}
	m.Dir = "/nonexistent"
	warnings := CheckPaths(m)
	if len(warnings) == 0 {
		t.Error("CheckPaths devrait signaler les fichiers manquants")
	}
}
