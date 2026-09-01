package library

import (
	"path/filepath"
	"testing"
)

func testLibraryDirectory(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "libraries"))
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRepositoryLoadsEightTemplates(t *testing.T) {
	repository := NewRepository(testLibraryDirectory(t))
	catalog, err := repository.Refresh()
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Errors) != 0 {
		t.Fatalf("load errors: %+v", catalog.Errors)
	}
	if len(catalog.Libraries) != 1 {
		t.Fatalf("libraries=%d, want 1", len(catalog.Libraries))
	}
	if len(catalog.Templates) != 8 {
		t.Fatalf("templates=%d, want 8", len(catalog.Templates))
	}
	expected := map[string][3]int{
		"17509": {33, 25, 1},
		"17510": {7, 0, 2},
		"18402": {31, 27, 1},
		"18555": {29, 20, 11},
		"18556": {44, 35, 1},
		"18558": {44, 37, 5},
		"19962": {45, 32, 32},
		"19963": {34, 27, 8},
	}
	for _, item := range catalog.Templates {
		counts, ok := expected[item.ID]
		if !ok {
			t.Fatalf("unexpected template %s", item.ID)
		}
		if item.BlockCount != counts[0] || item.LinkCount != counts[1] || item.GraphicCount != counts[2] {
			t.Errorf("template %s counts=%d/%d/%d want %d/%d/%d", item.ID, item.BlockCount, item.LinkCount, item.GraphicCount, counts[0], counts[1], counts[2])
		}
		if item.PrimitiveCount != counts[0]+counts[1]+counts[2] {
			t.Errorf("template %s primitive count mismatch", item.ID)
		}
	}
}

func TestShiftPointsPreservesBranchMarker(t *testing.T) {
	input := "(380,600);*(380,600);(380,670);(590,670);"
	got, err := ShiftPoints(input, 300, 100)
	if err != nil {
		t.Fatal(err)
	}
	want := "(680,700);*(680,700);(680,770);(890,770);"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestEmptyPrimitiveIVPresenceIsPreserved(t *testing.T) {
	repository := NewRepository(testLibraryDirectory(t))
	catalog, err := repository.Refresh()
	if err != nil {
		t.Fatal(err)
	}
	var key string
	for _, item := range catalog.Templates {
		if item.ID == "19962" {
			key = item.Key
		}
	}
	ref, ok := repository.Resolve(key)
	if !ok {
		t.Fatal("template 19962 not found")
	}
	found := 0
	for _, primitive := range ref.Template.Contents.Primitives {
		if primitive.ObjectType == "37" && primitive.CardID == "0" {
			found++
			if primitive.InitialValue == nil || *primitive.InitialValue != "" {
				t.Errorf("primitive %s: empty IV presence lost", primitive.ID)
			}
		}
	}
	if found != 4 {
		t.Fatalf("cardless GT37=%d, want 4", found)
	}
}
