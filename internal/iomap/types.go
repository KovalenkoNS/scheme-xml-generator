package iomap

// Plan is the physical inventory read from IO, not the derived ST mapping.
type Plan struct {
	SheetName   string       `json:"sheetName"`
	Controllers []Controller `json:"controllers"`
	RowCount    int          `json:"rowCount"`
	ModuleCount int          `json:"moduleCount"`
	SignalCount int          `json:"signalCount"`
	Warnings    []string     `json:"warnings"`
}

type Controller struct {
	Key       string   `json:"key"`
	SourceFCS string   `json:"sourceFcs"`
	Name      string   `json:"name"`
	Cabinet   string   `json:"cabinet"`
	Racks     []Rack   `json:"racks"`
	Modules   []Module `json:"modules"`
}

type Rack struct {
	Name  string `json:"name"`
	Panel string `json:"panel"` // front or back, based on the demonstrated chassis convention
	Order int    `json:"order"` // vertical position within the panel, zero based
}

type Module struct {
	Name     string    `json:"name"`
	Rack     string    `json:"rack"`
	Slot     int       `json:"slot"`
	Type     string    `json:"type"` // AI16H, AOC4H, DI32, DO32P
	Capacity int       `json:"capacity"`
	Channels []Channel `json:"channels"`
}

type Channel struct {
	Channel       int    `json:"channel"`
	Tag           string `json:"tag"`
	ObjectType    string `json:"objectType"`
	Reserve       bool   `json:"reserve"` // free channel, not a redundant physical placement
	Redundant     bool   `json:"redundant"`
	SourceRow     int    `json:"sourceRow"`
	SourceTag     string `json:"sourceTag"`
	BindingSource string `json:"bindingSource"`
}

type Selection struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}
