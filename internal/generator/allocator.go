package generator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"scheme-xml-generator/internal/config"
)

type allocatorState struct {
	NextT11  int64 `json:"nextT11"`
	NextCard int64 `json:"nextCard"`
	NextPOU  int64 `json:"nextPou"`
	NextPage int64 `json:"nextPage"`
}

type Allocator struct {
	path  string
	mu    sync.Mutex
	state allocatorState
}

// ReservationOptions connects manual transport IDs to the persistent
// allocator. Explicit ranges below the current cursor remain user-managed;
// ranges above it advance the cursor so the next automatic document cannot
// reuse them.
type ReservationOptions struct {
	T11Start  *int64
	CardStart *int64
	POUIDs    []*int64
}

func NewAllocator(path string, defaults config.IDDefaults) (*Allocator, error) {
	if defaults.NextPage == 0 {
		defaults.NextPage = config.Default().IDs.NextPage
	}
	// Unmarshalling a legacy state without nextPage preserves this new default;
	// existing T11/card/POU cursors are never reset during migration.
	allocator := &Allocator{path: path, state: allocatorState{NextT11: defaults.NextT11, NextCard: defaults.NextCard, NextPOU: defaults.NextPOU, NextPage: defaults.NextPage}}
	data, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, &allocator.state); err != nil {
			return nil, fmt.Errorf("разобрать state.json: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("прочитать state.json: %w", err)
	}
	if allocator.state.NextT11 < 1 || allocator.state.NextCard < 1 || allocator.state.NextPOU < 1 ||
		allocator.state.NextPage < 1 || allocator.state.NextT11 > maxTransportID+1 || allocator.state.NextCard > maxTransportID+1 || allocator.state.NextPOU > maxTransportID+1 || allocator.state.NextPage > maxTransportID+1 {
		return nil, fmt.Errorf("state.json содержит ID вне диапазона 1..%d", maxTransportID+1)
	}
	return allocator, nil
}

func (a *Allocator) Reserve(t11Count, cardCount int) (IDRange, error) {
	return a.ReserveMany(t11Count, cardCount, 1)
}

// ReserveMany reserves document-wide ranges for generated primitives, cards
// and automatic POU IDs. The state is committed only after the replacement
// state file has been written successfully.
func (a *Allocator) ReserveMany(t11Count, cardCount, pouCount int) (IDRange, error) {
	return a.WithReservation(t11Count, cardCount, pouCount, ReservationOptions{}, nil)
}

// WithReservation holds the allocator lock while consume validates and builds
// a document, then commits the state only when consume succeeds. This prevents
// invalid requests from consuming IDs and serializes concurrent generations.
func (a *Allocator) WithReservation(t11Count, cardCount, pouCount int, options ReservationOptions, consume func(IDRange) error) (IDRange, error) {
	if t11Count < 0 || cardCount < 0 || pouCount < 1 {
		return IDRange{}, fmt.Errorf("число T11ID и cardId должно быть неотрицательным, число POU — положительным")
	}
	if len(options.POUIDs) > pouCount {
		return IDRange{}, fmt.Errorf("число ручных POU ID превышает число POU")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	result := IDRange{T11Start: a.state.NextT11, CardStart: a.state.NextCard, POUID: a.state.NextPOU}
	if options.T11Start != nil {
		result.T11Start = *options.T11Start
	}
	if options.CardStart != nil {
		result.CardStart = *options.CardStart
	}
	next := a.state
	var err error
	t11End, err := addTransportCount(result.T11Start, t11Count, "T11ID")
	if err != nil {
		return IDRange{}, err
	}
	cardEnd, err := addTransportCount(result.CardStart, cardCount, "cardId")
	if err != nil {
		return IDRange{}, err
	}
	next.NextT11 = max(next.NextT11, t11End)
	next.NextCard = max(next.NextCard, cardEnd)

	usedPOUIDs := make(map[int64]struct{}, pouCount)
	for index := 0; index < pouCount; index++ {
		var pouID int64
		if index < len(options.POUIDs) && options.POUIDs[index] != nil {
			pouID = *options.POUIDs[index]
		} else {
			if result.POUID > maxTransportID-int64(index) {
				return IDRange{}, fmt.Errorf("диапазон POU ID выходит за signed 32-bit")
			}
			pouID = result.POUID + int64(index)
		}
		pouEnd, rangeErr := addTransportCount(pouID, 1, "POU ID")
		if rangeErr != nil {
			return IDRange{}, rangeErr
		}
		if _, duplicate := usedPOUIDs[pouID]; duplicate {
			return IDRange{}, fmt.Errorf("POU ID %d повторяется в резервировании", pouID)
		}
		usedPOUIDs[pouID] = struct{}{}
		next.NextPOU = max(next.NextPOU, pouEnd)
	}
	if consume != nil {
		if err := consume(result); err != nil {
			return IDRange{}, err
		}
	}
	if err := a.persist(next); err != nil {
		return IDRange{}, &AllocatorPersistenceError{Err: err}
	}
	a.state = next
	return result, nil
}

// WithDiagnosticReservation shares primitive/card cursors with other exports,
// but advances only the dedicated page cursor, leaving every POU ID untouched.
// Preparation, validation and serialization all run before state is committed.
func (a *Allocator) WithDiagnosticReservation(t11Count, cardCount, pageCount int, consume func(DiagnosticIDRange) error) (DiagnosticIDRange, error) {
	if t11Count < 0 || cardCount < 0 || pageCount < 1 {
		return DiagnosticIDRange{}, fmt.Errorf("число примитивов и карточек должно быть неотрицательным, число кадров — положительным")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	result := DiagnosticIDRange{T11Start: a.state.NextT11, CardStart: a.state.NextCard, PageStart: a.state.NextPage}
	next := a.state
	var err error
	if next.NextT11, err = addTransportCount(result.T11Start, t11Count, "SourceT11ID"); err != nil {
		return DiagnosticIDRange{}, err
	}
	if next.NextCard, err = addTransportCount(result.CardStart, cardCount, "CardID"); err != nil {
		return DiagnosticIDRange{}, err
	}
	if next.NextPage, err = addTransportCount(result.PageStart, pageCount, "PageID"); err != nil {
		return DiagnosticIDRange{}, err
	}
	if consume != nil {
		if err := consume(result); err != nil {
			return DiagnosticIDRange{}, err
		}
	}
	if err := a.persist(next); err != nil {
		return DiagnosticIDRange{}, &AllocatorPersistenceError{Err: err}
	}
	a.state = next
	return result, nil
}

// AllocatorPersistenceError identifies an infrastructure failure while
// committing state.json. Callers should report it as a server error even when
// the request contains manual IDs.
type AllocatorPersistenceError struct {
	Err error
}

func (e *AllocatorPersistenceError) Error() string {
	return fmt.Sprintf("сохранить состояние ID: %v", e.Err)
}

func (e *AllocatorPersistenceError) Unwrap() error {
	return e.Err
}

func (a *Allocator) persist(next allocatorState) error {
	if err := os.MkdirAll(filepath.Dir(a.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	temp := a.path + ".tmp"
	if err := os.WriteFile(temp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.Rename(temp, a.path); err != nil {
		_ = os.Remove(temp)
		return err
	}
	return nil
}

func addTransportCount(start int64, count int, label string) (int64, error) {
	if count == 0 {
		if start < 1 || start > maxTransportID+1 {
			return 0, fmt.Errorf("начальный %s находится вне signed 32-bit", label)
		}
		return start, nil
	}
	if start < 1 || start > maxTransportID || int64(count-1) > maxTransportID-start {
		return 0, fmt.Errorf("диапазон %s выходит за signed 32-bit", label)
	}
	return start + int64(count), nil
}
