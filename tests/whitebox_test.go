// Package tests connects separately stored private-contract suites to real Go packages.
package tests

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestWhitebox executes historical private-contract tests without moving production code.
// It overlays only test sources, tracks source/fixture contents for the parent cache,
// and invokes subject packages explicitly so the driver cannot call itself recursively.
func TestWhitebox(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	digest := trackInputs(t, root)
	temporary := t.TempDir()
	replacements := map[string]string{}
	packages := map[string]bool{}
	rootWhitebox := filepath.Join(root, "tests", "whitebox")
	testCount := 0
	err = filepath.WalkDir(rootWhitebox, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relative, relErr := filepath.Rel(rootWhitebox, path)
		if relErr != nil {
			return relErr
		}
		production := filepath.Join(root, relative)
		if _, statErr := os.Stat(production); !os.IsNotExist(statErr) {
			return fmt.Errorf("overlay must add a test, not replace an existing source: %s", production)
		}
		if _, statErr := os.Stat(filepath.Dir(production)); statErr != nil {
			return fmt.Errorf("test has no production package: %s: %w", relative, statErr)
		}
		replacements[production] = path
		packages[filepath.Dir(relative)] = true
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		testCount += strings.Count(string(data), "func Test")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(packages) == 0 || testCount == 0 {
		t.Fatal("whitebox inventory is empty")
	}
	paths := make([]string, 0, len(packages))
	for relative := range packages {
		paths = append(paths, "./"+filepath.ToSlash(relative))
		if strings.HasPrefix(filepath.ToSlash(relative), "internal/generator/") {
			// Existing generator fixtures use the former package cwd. The production
			// package still has its own source identity and private symbol visibility.
			packageName := filepath.Base(relative)
			mainPath := filepath.Join(temporary, "main_"+packageName+"_test.go")
			main := fmt.Sprintf("package %s\nimport(\"os\";\"testing\")\nfunc TestMain(m *testing.M){if err:=os.Chdir(%q);err!=nil{panic(err)};os.Exit(m.Run())}\n", packageName, filepath.Join(root, "internal", "generator"))
			if err := os.WriteFile(mainPath, []byte(main), 0600); err != nil {
				t.Fatal(err)
			}
			replacements[filepath.Join(root, relative, "whitebox_main_test.go")] = mainPath
		}
	}
	sort.Strings(paths)
	data, err := json.Marshal(struct{ Replace map[string]string }{replacements})
	if err != nil {
		t.Fatal(err)
	}
	overlay := filepath.Join(temporary, "overlay.json")
	if err := os.WriteFile(overlay, data, 0600); err != nil {
		t.Fatal(err)
	}
	args := append([]string{"test", "-p", "1", "-count=1", "-overlay", overlay}, paths...)
	if testing.Verbose() {
		// Preserve child PASS/SKIP evidence when the canonical run explicitly requests it.
		args = append(args[:1], append([]string{"-v"}, args[1:]...)...)
	}
	command := exec.Command("go", args...)
	command.Dir = root
	output, err := command.CombinedOutput()
	t.Logf("whitebox: %d packages, %d test declarations; input SHA256 %s\n%s", len(paths), testCount, digest, output)
	if err != nil {
		t.Fatalf("subject test process failed: %v", err)
	}
}

// trackInputs makes the outer Go cache depend on the files consumed by child tests.
// It reads production, separate tests, embedded assets and optional user fixtures;
// changing contents or adding/removing a directory entry invalidates the cached run.
func trackInputs(t *testing.T, root string) string {
	t.Helper()
	hash := sha256.New()
	for _, name := range []string{"go.mod", "go.sum", "internal", "cmd", "web", "tools", "tests", "libraries", "output", "XML dev", "config.example.json"} {
		path := filepath.Join(root, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}
		err := filepath.WalkDir(path, func(file string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				return nil
			}
			data, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			_, _ = hash.Write([]byte(file))
			_, _ = hash.Write(data)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return hex.EncodeToString(hash.Sum(nil))
}
