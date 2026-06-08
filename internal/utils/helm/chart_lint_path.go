package helm

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// lintChartPath adapts chart paths for helm lint.
// Helm v4 LocateChart may return a content-cache "*.chart" archive; helm lint only
// auto-extracts "*.tgz" / "*.tar.gz" and otherwise treats the path as a directory.
func lintChartPath(chartPath string) (path string, cleanup func(), err error) {
	cleanup = func() {}
	if !strings.HasSuffix(chartPath, ".chart") {
		return chartPath, cleanup, nil
	}

	tmp, err := os.CreateTemp("", "helm-lint-*.tgz")
	if err != nil {
		return "", cleanup, fmt.Errorf("create temp chart for lint: %w", err)
	}
	cleanup = func() { _ = os.Remove(tmp.Name()) }

	src, err := os.Open(chartPath)
	if err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("open chart archive: %w", err)
	}
	defer src.Close()

	if _, err := io.Copy(tmp, src); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("copy chart archive for lint: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return tmp.Name(), cleanup, nil
}
