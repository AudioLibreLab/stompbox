package render

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/AudioLibreLab/stompbox/internal/manifest"
)

var update = flag.Bool("update", false, "rewrite golden files")

// TestRenderGolden compares the full render of the test manifest against
// the reference files. Regenerate with: go test ./internal/render -update
func TestRenderGolden(t *testing.T) {
	m, err := manifest.Load("testdata/stompbox.yaml")
	if err != nil {
		t.Fatal(err)
	}
	// Fixed directory so resolved absolute paths are independent of
	// the repo's location.
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
			t.Errorf("%s: missing golden file (run go test -update): %v", f.Path, err)
			continue
		}
		if f.Content != string(want) {
			t.Errorf("%s: render differs from golden\n--- want ---\n%s\n--- got ---\n%s", f.Path, want, f.Content)
		}
	}

	// Detect orphaned golden files (artifact removed from the render).
	if !*update {
		err := filepath.Walk("testdata/golden", func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return err
			}
			rel, _ := filepath.Rel("testdata/golden", path)
			if !seen[rel] {
				t.Errorf("orphaned golden: %s is no longer generated", rel)
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
		t.Error("CheckPaths should report missing files")
	}
}
