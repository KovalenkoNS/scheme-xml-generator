// Package fixtures reads optional development samples without copying them into test packages.
package fixtures

import (
	"os"
	"path/filepath"
	"testing"
)

// ReadDevelopment locates the module root from the test cwd and reads its XML dev sample.
// Missing private samples are an explicit skipped scenario; other read failures fail the test.
func ReadDevelopment(t testing.TB, name string) []byte {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Fatal("module root not found for development fixture")
		}
		root = parent
	}
	data, err := os.ReadFile(filepath.Join(root, "XML dev", name))
	if os.IsNotExist(err) {
		t.Skipf("development fixture %s not installed", name)
	}
	if err != nil {
		t.Fatal(err)
	}
	return data
}
