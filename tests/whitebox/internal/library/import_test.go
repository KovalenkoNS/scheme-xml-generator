// Import contract checks cover byte preservation, type discovery and rollback.
package library

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const typeOnlyLibrary = `<root><SCADATA_VER VER="29"/><SECTION Num="2"><OTHER><OBJTYPE ID="100" Name="Imported"><ISAOBJLIST><ISAOBJ ID="101" Prefix="ready"><ISATNAME>BOOL</ISATNAME><LIBNAME>standard</LIBNAME><KINDOBJ>1</KINDOBJ></ISAOBJ></ISAOBJLIST><CHILD><OBJTYPE ID="200" Name="Child"/></CHILD></OBJTYPE></OTHER></SECTION></root>`

// Проверяет импорт библиотеки только с типами: байты и поля сохраняются, повторный импорт переиспользует тот же файл.
func TestImportPreservesBytesAndExposesTypesWithoutTemplates(t *testing.T) {
	directory := t.TempDir()
	repository := NewRepository(directory)
	result, err := repository.Import(strings.NewReader(typeOnlyLibrary))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Created || len(result.Catalog.Templates) != 0 || len(result.Catalog.Types) != 2 || result.Catalog.Libraries[0].TypeCount != 2 {
		t.Fatalf("unexpected imported catalog: %+v", result)
	}
	field := result.Catalog.Types[0].Fields[0]
	if field.Name != "ready" || field.TypeName != "BOOL" || field.LibraryName != "standard" || field.Kind != "1" {
		t.Fatalf("type information lost: %+v", field)
	}
	data, err := os.ReadFile(filepath.Join(directory, result.File))
	if err != nil || !bytes.Equal(data, []byte(typeOnlyLibrary)) {
		t.Fatalf("source bytes changed: %v", err)
	}
	before, _ := os.Stat(filepath.Join(directory, result.File))
	again, err := repository.Import(strings.NewReader(typeOnlyLibrary))
	if err != nil || again.Created || again.File != result.File {
		t.Fatalf("repeated import should reuse the same file: %+v, %v", again, err)
	}
	after, _ := os.Stat(filepath.Join(directory, result.File))
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("repeated import rewrote the original")
	}
	result.Catalog.Types[0].Fields[0].TypeName = "changed"
	if repository.Catalog().Types[0].Fields[0].TypeName != "BOOL" {
		t.Fatal("catalog snapshot exposes mutable repository state")
	}
	entries, _ := os.ReadDir(directory)
	if len(entries) != 1 {
		t.Fatalf("temporary files leaked: %v", entries)
	}
}

// Подаёт импорту библиотеки пустой и повреждённый XML; уже сохранённые файлы и каталог должны остаться неизменными.
func TestInvalidImportLeavesFilesAndCatalogUnchanged(t *testing.T) {
	for name, input := range map[string]string{
		"empty": "", "malformed": `<root><SECTION>`, "wrong-root": `<library/>`,
		"no-types": `<root/>`, "second-root": typeOnlyLibrary + `<root/>`,
		"trailing-garbage": typeOnlyLibrary + `unexpected`,
	} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			repository := NewRepository(directory)
			good, err := repository.Import(strings.NewReader(typeOnlyLibrary))
			if err != nil {
				t.Fatal(err)
			}
			_, err = repository.Import(strings.NewReader(input))
			if !errors.Is(err, ErrInvalidLibrary) {
				t.Fatalf("got %v, want invalid library", err)
			}
			entries, _ := os.ReadDir(directory)
			if len(entries) != 1 || entries[0].Name() != good.File || !reflect.DeepEqual(good.Catalog, repository.Catalog()) {
				t.Fatal("failed import changed existing files or catalog")
			}
		})
	}
}

// Проверяет конфликт имени импортируемой библиотеки с пользовательским файлом: операция отклоняется без перезаписи.
func TestImportNeverOverwritesConflictingFile(t *testing.T) {
	directory := t.TempDir()
	hash := sha256.Sum256([]byte(typeOnlyLibrary))
	name := "import-" + hex.EncodeToString(hash[:]) + ".xml"
	path := filepath.Join(directory, name)
	original := []byte(`user-owned file`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	repository := NewRepository(directory)
	if _, err := repository.Import(strings.NewReader(typeOnlyLibrary)); !errors.Is(err, ErrInvalidLibrary) {
		t.Fatalf("conflict should fail without replacement: %v", err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(after, original) {
		t.Fatal("user-owned file was overwritten")
	}
}
