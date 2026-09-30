// Диапазоны и пределы ID XML-экспорта: вход для резервирования и предметных генераторов.
package identity

type DocumentRequirements struct {
	T11Count    int `json:"t11Count"`
	CardCount   int `json:"cardCount"`
	POUCount    int `json:"pouCount"`
	SignalCount int `json:"signalCount"`
}

type IDRange struct {
	T11Start  int64
	CardStart int64
	POUID     int64
}

// DiagnosticIDRange belongs to operator-panel pages, not program POUs.
type DiagnosticIDRange struct {
	T11Start  int64
	CardStart int64
	PageStart int64
}

const MaxTransportID = int64(2147483647)
