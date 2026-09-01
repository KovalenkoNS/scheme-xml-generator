package generator

import (
	"path/filepath"
	"testing"

	"scheme-xml-generator/internal/config"
)

func TestAllocatorPersistsAndAdvances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "state.json")
	defaults := config.IDDefaults{NextT11: 1000, NextCard: 2000, NextPOU: 3000}
	allocator, err := NewAllocator(path, defaults)
	if err != nil {
		t.Fatal(err)
	}
	first, err := allocator.Reserve(10, 3)
	if err != nil {
		t.Fatal(err)
	}
	second, err := allocator.Reserve(5, 2)
	if err != nil {
		t.Fatal(err)
	}
	if first != (IDRange{T11Start: 1000, CardStart: 2000, POUID: 3000}) {
		t.Fatalf("first=%+v", first)
	}
	if second != (IDRange{T11Start: 1010, CardStart: 2003, POUID: 3001}) {
		t.Fatalf("second=%+v", second)
	}
	reloaded, err := NewAllocator(path, defaults)
	if err != nil {
		t.Fatal(err)
	}
	third, err := reloaded.Reserve(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if third != (IDRange{T11Start: 1015, CardStart: 2005, POUID: 3002}) {
		t.Fatalf("third=%+v", third)
	}
}
