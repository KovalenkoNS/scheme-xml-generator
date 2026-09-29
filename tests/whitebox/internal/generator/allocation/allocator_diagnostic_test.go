// HMI page/primitive allocation checks for legacy-state migration and atomic persistence.
package allocation

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"scheme-xml-generator/internal/config"
	"scheme-xml-generator/internal/generator/contracts"
	"testing"
)

// TestDiagnosticAllocatorMigratesLegacyStateAndKeepsPOUCursor migrates legacy state to HMI page allocation and
// checks failed callbacks preserve bytes while HMI and FBD cursors stay independent.
func TestDiagnosticAllocatorMigratesLegacyStateAndKeepsPOUCursor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	legacy := []byte(`{"nextT11":1234,"nextCard":2345,"nextPou":3456}`)
	if err := os.WriteFile(path, legacy, 0o644); err != nil {
		t.Fatal(err)
	}
	allocator, err := NewAllocator(path, config.Default().IDs)
	if err != nil {
		t.Fatal(err)
	}
	wanted := contracts.DiagnosticIDRange{T11Start: 1234, CardStart: 2345, PageStart: 1000000}
	failed := errors.New("invalid diagnostic document")
	if _, err := allocator.WithDiagnosticReservation(10, 6, 2, func(ids contracts.DiagnosticIDRange) error {
		if ids != wanted {
			t.Fatalf("legacy cursor lost: %+v", ids)
		}
		return failed
	}); !errors.Is(err, failed) {
		t.Fatalf("expected callback error: %v", err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != string(legacy) {
		t.Fatalf("invalid generation modified legacy state: %s %v", data, err)
	}
	if actual, err := allocator.WithDiagnosticReservation(10, 6, 2, nil); err != nil || actual != wanted {
		t.Fatalf("first reservation %+v %v", actual, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var state AllocatorState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	if state != (AllocatorState{NextT11: 1244, NextCard: 2351, NextPOU: 3456, NextPage: 1000002}) {
		t.Fatalf("unexpected diagnostic state %+v", state)
	}
	reloaded, err := NewAllocator(path, config.Default().IDs)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := reloaded.ReserveMany(1, 1, 1)
	if err != nil || ids != (contracts.IDRange{T11Start: 1244, CardStart: 2351, POUID: 3456}) {
		t.Fatalf("diagnostics consumed POU IDs: %+v %v", ids, err)
	}
	diagnostic, err := reloaded.WithDiagnosticReservation(5, 4, 1, nil)
	if err != nil || diagnostic != (contracts.DiagnosticIDRange{T11Start: 1245, CardStart: 2352, PageStart: 1000002}) {
		t.Fatalf("FBD consumed page IDs: %+v %v", diagnostic, err)
	}
}

// TestDiagnosticAllocatorRejectsInvalidRangesAndPersistsAtomically rejects invalid HMI counts/overflow and checks
// storage failure cannot commit page or primitive cursors.
func TestDiagnosticAllocatorRejectsInvalidRangesAndPersistsAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	defaults := config.IDDefaults{NextT11: contracts.MaxTransportID, NextCard: contracts.MaxTransportID, NextPOU: contracts.MaxTransportID + 1, NextPage: contracts.MaxTransportID}
	allocator, err := NewAllocator(path, defaults)
	if err != nil {
		t.Fatal(err)
	}
	for _, counts := range [][3]int{{-1, 1, 1}, {1, -1, 1}, {0, 0, 0}, {2, 1, 1}, {1, 2, 1}, {1, 1, 2}} {
		if _, err := allocator.WithDiagnosticReservation(counts[0], counts[1], counts[2], nil); err == nil {
			t.Fatalf("invalid count/range accepted: %v", counts)
		}
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("invalid request wrote state: %v", err)
	}
	// Exhausted program POU cursor does not prevent operator-panel exports.
	ids, err := allocator.WithDiagnosticReservation(1, 1, 1, nil)
	if err != nil || ids.PageStart != contracts.MaxTransportID {
		t.Fatalf("last page not available: %+v %v", ids, err)
	}
	if _, err := allocator.WithDiagnosticReservation(0, 0, 1, nil); err == nil {
		t.Fatal("exhausted page cursor accepted")
	}
	brokenPath := filepath.Join(t.TempDir(), "state.json")
	broken, err := NewAllocator(brokenPath, config.Default().IDs)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(brokenPath+".tmp", 0o755); err != nil {
		t.Fatal(err)
	}
	before := broken.state
	_, err = broken.WithDiagnosticReservation(5, 4, 1, nil)
	var persistence *AllocatorPersistenceError
	if !errors.As(err, &persistence) || broken.state != before {
		t.Fatalf("persistence failure changed cursor: %v %+v", err, broken.state)
	}
}
