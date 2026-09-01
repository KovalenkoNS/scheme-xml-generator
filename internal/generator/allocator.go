package generator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"scheme-xml-generator/internal/config"
	"sync"
)

type allocatorState struct {
	NextT11  int64 `json:"nextT11"`
	NextCard int64 `json:"nextCard"`
	NextPOU  int64 `json:"nextPou"`
}

type Allocator struct {
	path  string
	mu    sync.Mutex
	state allocatorState
}

func NewAllocator(path string, defaults config.IDDefaults) (*Allocator, error) {
	allocator := &Allocator{path: path, state: allocatorState{NextT11: defaults.NextT11, NextCard: defaults.NextCard, NextPOU: defaults.NextPOU}}
	data, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, &allocator.state); err != nil {
			return nil, fmt.Errorf("разобрать state.json: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("прочитать state.json: %w", err)
	}
	if allocator.state.NextT11 < 1 || allocator.state.NextCard < 1 || allocator.state.NextPOU < 1 {
		return nil, fmt.Errorf("state.json содержит неположительные ID")
	}
	return allocator, nil
}

func (a *Allocator) Reserve(t11Count, cardCount int) (IDRange, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	result := IDRange{T11Start: a.state.NextT11, CardStart: a.state.NextCard, POUID: a.state.NextPOU}
	a.state.NextT11 += int64(t11Count)
	a.state.NextCard += int64(cardCount)
	a.state.NextPOU++
	if err := os.MkdirAll(filepath.Dir(a.path), 0o755); err != nil {
		return IDRange{}, err
	}
	data, err := json.MarshalIndent(a.state, "", "  ")
	if err != nil {
		return IDRange{}, err
	}
	temp := a.path + ".tmp"
	if err := os.WriteFile(temp, append(data, '\n'), 0o644); err != nil {
		return IDRange{}, err
	}
	if err := os.Rename(temp, a.path); err != nil {
		_ = os.Remove(temp)
		return IDRange{}, err
	}
	return result, nil
}
