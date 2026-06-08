package helm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"helm.sh/helm/v4/pkg/action"
)

func TestLintChartPath_dotChartArchive(t *testing.T) {
	cachePath := "/home/cernet/.cache/helm/content/69/69cd6e158da9f6584928954a38708d67a177985cd2dda04da9e90c2677d73adb.chart"
	if _, err := os.Stat(cachePath); err != nil {
		t.Skip("helm cache chart not present:", err)
	}

	lintPath, cleanup, err := lintChartPath(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	if !strings.HasSuffix(lintPath, ".tgz") {
		t.Fatalf("lint path = %q, want .tgz suffix", lintPath)
	}

	lint := action.NewLint()
	result := lint.Run([]string{lintPath}, map[string]any{})
	if len(result.Errors) > 0 {
		t.Fatalf("lint errors: %v", result.Errors)
	}
}

func TestLintChartPath_plainDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Chart.yaml"), []byte("apiVersion: v2\nname: test\nversion: 0.1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	path, cleanup, err := lintChartPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if path != dir {
		t.Fatalf("path = %q, want %q", path, dir)
	}
}
