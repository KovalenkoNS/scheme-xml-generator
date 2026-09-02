package generator

import (
	"errors"
	"os"
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

func TestAllocatorReserveManyAdvancesAllDocumentRanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "state.json")
	defaults := config.IDDefaults{NextT11: 1000, NextCard: 2000, NextPOU: 3000}
	allocator, err := NewAllocator(path, defaults)
	if err != nil {
		t.Fatal(err)
	}
	first, err := allocator.ReserveMany(270, 90, 2)
	if err != nil {
		t.Fatal(err)
	}
	if first != (IDRange{T11Start: 1000, CardStart: 2000, POUID: 3000}) {
		t.Fatalf("first=%+v", first)
	}
	reloaded, err := NewAllocator(path, defaults)
	if err != nil {
		t.Fatal(err)
	}
	second, err := reloaded.ReserveMany(18, 6, 1)
	if err != nil {
		t.Fatal(err)
	}
	if second != (IDRange{T11Start: 1270, CardStart: 2090, POUID: 3002}) {
		t.Fatalf("second=%+v", second)
	}
	if _, err := reloaded.ReserveMany(-1, 0, 1); err == nil {
		t.Fatal("negative reservation was accepted")
	}
	if _, err := reloaded.ReserveMany(0, 0, 0); err == nil {
		t.Fatal("zero POU reservation was accepted")
	}
}

func TestAllocatorRejectsSigned32OverflowWithoutAdvancing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	defaults := config.IDDefaults{NextT11: maxTransportID, NextCard: maxTransportID, NextPOU: maxTransportID}
	allocator, err := NewAllocator(path, defaults)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := allocator.ReserveMany(2, 1, 1); err == nil {
		t.Fatal("signed 32-bit overflow was accepted")
	}
	ids, err := allocator.ReserveMany(1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if ids != (IDRange{T11Start: maxTransportID, CardStart: maxTransportID, POUID: maxTransportID}) {
		t.Fatalf("allocator advanced after rejected reservation: %+v", ids)
	}
	if _, err := allocator.ReserveMany(0, 0, 1); err == nil {
		t.Fatal("exhausted POU range was accepted")
	}
}

func TestAllocatorCommitsOnlySuccessfulGenerationAndTracksManualRanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	allocator, err := NewAllocator(path, config.IDDefaults{NextT11: 1000, NextCard: 2000, NextPOU: 3000})
	if err != nil {
		t.Fatal(err)
	}
	manualT11, manualCard, manualPOU := int64(5000), int64(6000), int64(7000)
	options := ReservationOptions{T11Start: &manualT11, CardStart: &manualCard, POUIDs: []*int64{&manualPOU}}
	wantFailure := errors.New("generation failed")
	if _, err := allocator.WithReservation(2, 1, 1, options, func(IDRange) error { return wantFailure }); !errors.Is(err, wantFailure) {
		t.Fatalf("callback error=%v want %v", err, wantFailure)
	}
	var used IDRange
	if _, err := allocator.WithReservation(2, 1, 1, options, func(ids IDRange) error {
		used = ids
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if used.T11Start != manualT11 || used.CardStart != manualCard || used.POUID != 3000 {
		t.Fatalf("manual reservation=%+v", used)
	}
	next, err := allocator.ReserveMany(1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if next != (IDRange{T11Start: 5002, CardStart: 6001, POUID: 7001}) {
		t.Fatalf("next automatic range=%+v", next)
	}
}

func TestAllocatorAllowsManualPOUAfterAutomaticRangeIsExhausted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	allocator, err := NewAllocator(path, config.IDDefaults{NextT11: 1000, NextCard: 2000, NextPOU: maxTransportID})
	if err != nil {
		t.Fatal(err)
	}
	lastPOU := maxTransportID
	if _, err := allocator.WithReservation(0, 0, 1, ReservationOptions{POUIDs: []*int64{&lastPOU}}, nil); err != nil {
		t.Fatal(err)
	}
	manualPOU := int64(42)
	ids, err := allocator.WithReservation(0, 0, 1, ReservationOptions{POUIDs: []*int64{&manualPOU}}, nil)
	if err != nil {
		t.Fatalf("manual POU below an exhausted automatic cursor was rejected: %v", err)
	}
	if ids.POUID != maxTransportID+1 {
		t.Fatalf("automatic cursor=%d want %d", ids.POUID, maxTransportID+1)
	}
	if _, err := allocator.ReserveMany(0, 0, 1); err == nil {
		t.Fatal("exhausted automatic POU range was accepted")
	}
}

func TestAllocatorValidatesOnlyActuallyUsedAutomaticPOUIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	allocator, err := NewAllocator(path, config.IDDefaults{NextT11: 1000, NextCard: 2000, NextPOU: maxTransportID})
	if err != nil {
		t.Fatal(err)
	}
	manualPOU := int64(1)
	if _, err := allocator.WithReservation(0, 0, 2, ReservationOptions{POUIDs: []*int64{nil, &manualPOU}}, nil); err != nil {
		t.Fatalf("valid mixed automatic/manual POU IDs were rejected: %v", err)
	}
}

func TestAllocatorWrapsPersistenceFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	allocator, err := NewAllocator(path, config.IDDefaults{NextT11: 1000, NextCard: 2000, NextPOU: 3000})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path+".tmp", 0o755); err != nil {
		t.Fatal(err)
	}
	_, err = allocator.ReserveMany(1, 1, 1)
	var persistenceErr *AllocatorPersistenceError
	if !errors.As(err, &persistenceErr) {
		t.Fatalf("error=%v is not AllocatorPersistenceError", err)
	}
}
