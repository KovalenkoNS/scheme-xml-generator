package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"scheme-xml-generator/internal/config"
	"scheme-xml-generator/internal/library"
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
	physicalTemplates := make(map[string]*library.TemplateRef, 4)
	for index, item := range catalog.Templates {
		if item.LibraryFile != targetFile || !item.Supported {
			continue
		}
		ref, ok := repository.Resolve(item.Key)
		if !ok {
			failures = append(failures, fmt.Sprintf("%s/%s: reference missing", item.OwnerName, item.Name))
			continue
		}
		switch item.ID {
		case "17510", "17625", "18442", "18714":
			physicalTemplates[item.ID] = ref
		}
		result, generateErr := (Generator{Config: config.Default()}).Generate(ref, Request{
			ObjectName: fmt.Sprintf("_SMOKE_%04d", index+1),
			POUName:    fmt.Sprintf("SMOKE_POU_%04d", index+1),
			NameMode:   "base",
		}, IDRange{T11Start: nextT11, CardStart: nextCard, POUID: nextPOU})
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

	profiles := []struct {
		ioType     string
		templateID string
	}{
		{ioType: ioTypeAI, templateID: "17510"},
		{ioType: ioTypeAO, templateID: "17625"},
		{ioType: ioTypeDI, templateID: "18442"},
		{ioType: ioTypeDO, templateID: "18714"},
	}
	resolved := make([]ResolvedPOU, 0, len(profiles))
	for index, profile := range profiles {
		ref := physicalTemplates[profile.templateID]
		if ref == nil {
			t.Fatalf("physical %s template ID=%s not found in %s", profile.ioType, profile.templateID, targetFile)
		}
		moduleID := int64(index)
		request := POURequest{
			Name:               "PHYSICAL_" + profile.ioType,
			DefaultTemplateKey: ref.Key,
			IO: &IORequest{Type: profile.ioType, Modules: []IOModuleRequest{{
				ID:            &moduleID,
				BindingPrefix: fmt.Sprintf("_IO_SMOKE_%s", profile.ioType),
				InstanceName:  fmt.Sprintf("_IO_%s_%d", profile.ioType, index),
				Signals: []SignalRequest{{
					ObjectName: fmt.Sprintf("_PHYSICAL_%s_SIGNAL", profile.ioType),
					NameMode:   "base",
				}},
			}}},
		}
		resolved = append(resolved, resolvedIOPOU(request, ref))
	}
	physical, err := (Generator{Config: config.Default()}).GenerateDocument(
		Request{}, resolved,
		IDRange{T11Start: nextT11, CardStart: nextCard, POUID: nextPOU},
	)
	if err != nil {
		t.Fatalf("physical AI/AO/DI/DO document from %s: %v", targetFile, err)
	}
	if physical.Summary.POUCount != 4 || physical.Summary.SignalCount != 4 || physical.Summary.IOModuleCount != 4 {
		t.Fatalf("physical document summary=%+v", physical.Summary)
	}
}
