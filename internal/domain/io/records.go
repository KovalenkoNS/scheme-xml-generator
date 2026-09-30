// Package io хранит прочитанные сигналы и их физические размещения независимо от Excel и профиля SCADA.
package io

// Source связывает каждую прочитанную строку с исходником; число сигналов не равно числу резервных размещений.
type Source struct {
	Records  []Record      `json:"records"`
	Excluded []ExcludedRow `json:"excluded"`
}

type Location struct {
	Sheet string `json:"sheet"`
	Row   int    `json:"row"`
}

type Record struct {
	Location
	ControllerName     string      `json:"controllerName"`
	Kind               string      `json:"kind"`
	SignalName         string      `json:"signalName"`
	Loop               string      `json:"loop"`
	SignalType         string      `json:"signalType"`
	Description        string      `json:"description"`
	IOType             string      `json:"ioType"`
	Placements         []Placement `json:"placements"`
	Ranges             []Range     `json:"ranges,omitempty"`
	Reserve            bool        `json:"reserve"`
	ControllerID       string      `json:"controllerId,omitempty"`
	MarshallingCabinet string      `json:"marshallingCabinet,omitempty"`
}

type Placement struct {
	Module  string `json:"module"`
	Rack    string `json:"rack"`
	Slot    int    `json:"slot"`
	Channel int    `json:"channel"`
	Role    string `json:"role"`
}

// Range сохраняет исходные значения без подмены основной и дополнительной шкал друг другом.
type Range struct {
	Minimum string `json:"minimum"`
	Maximum string `json:"maximum"`
	Unit    string `json:"unit"`
}

type ExcludedRow struct {
	Location
	IOType string `json:"ioType"`
	Reason string `json:"reason"`
}
