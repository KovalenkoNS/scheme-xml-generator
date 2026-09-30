// Optional full-library FBD generation checks; retired physical augmentation assertions are recorded separately.
package fbd

import (
	"fmt"
	"os"
	"path/filepath"
	"scheme-xml-generator/internal/config"
	fbdrequest "scheme-xml-generator/internal/generator/fbd/request"
	xmlidentity "scheme-xml-generator/internal/generator/identity"
	"scheme-xml-generator/internal/library"
	"strings"
	"testing"
)

// This optional smoke test keeps the catalog's Supported flag honest for the
// large vendor fixture. The file is intentionally not required in a clean
// checkout, because production libraries are user-supplied and gitignored.
func TestEverySupportedSinopecTemplateGeneratesWhenPresent(t *testing.T) {
	libraryDirectory := filepath.Join("..", "..", "libraries")
	matches, err := filepath.Glob(filepath.Join(libraryDirectory, "all_lb_sinopec*.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Skip("all_lb_sinopec library is not present")
	}
	targetFile := filepath.Base(matches[0])
	if _, err := os.Stat(matches[0]); err != nil {
		t.Fatal(err)
	}

	repository := library.NewRepository(libraryDirectory)
	catalog, err := repository.Refresh()
	if err != nil {
		t.Fatal(err)
	}
	nextT11, nextCard, nextPOU := int64(10_000_000), int64(2_000_000), int64(300_000)
	failures := make([]string, 0)
	generated := 0
	for index, item := range catalog.Templates {
		if item.LibraryFile != targetFile || !item.Supported {
			continue
		}
		ref, ok := repository.Resolve(item.Key)
		if !ok {
			failures = append(failures, fmt.Sprintf("%s/%s: reference missing", item.OwnerName, item.Name))
			continue
		}
		result, generateErr := (Generator{Config: config.Default()}).Generate(ref, fbdrequest.Request{
			ObjectName: fmt.Sprintf("_SMOKE_%04d", index+1),
			POUName:    fmt.Sprintf("SMOKE_POU_%04d", index+1),
			NameMode:   "base",
		}, xmlidentity.IDRange{T11Start: nextT11, CardStart: nextCard, POUID: nextPOU})
		if generateErr != nil {
			failures = append(failures, fmt.Sprintf("%s/%s (ID=%s): %v", item.OwnerName, item.Name, item.ID, generateErr))
			continue
		}
		t11Count, cardCount := Requirements(ref)
		if result.Summary.Blocks+result.Summary.Links+result.Summary.Graphics != t11Count {
			failures = append(failures, fmt.Sprintf("%s/%s: summary count mismatch", item.OwnerName, item.Name))
			continue
		}
		nextT11 += int64(t11Count)
		nextCard += int64(cardCount)
		nextPOU++
		generated++
	}
	if len(failures) > 0 {
		t.Fatalf("%d templates marked supported did not generate:\n%s", len(failures), strings.Join(failures, "\n"))
	}
	if generated == 0 {
		t.Fatalf("no supported templates found in %s", targetFile)
	}

}
