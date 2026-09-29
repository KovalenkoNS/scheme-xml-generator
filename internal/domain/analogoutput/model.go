// Validated analog-output assignments and ranges shared by adapters and renderers.
package analogoutput

// Plan contains only validated assignments. Every module has channels 0 through
// 3, including named reserves for channels absent from the source map. Repeated
// object calls stay in these positions but are annotated as empty graphic slots.
type Plan struct {
	Groups          []Group  `json:"groups"`
	RowCount        int      `json:"rowCount"`
	GroupCount      int      `json:"groupCount"`
	ControllerCount int      `json:"fcsCount"` // fcsCount — прежнее имя HTTP-поля, не термин модели ПЛК.
	ModuleCount     int      `json:"moduleCount"`
	ChannelCount    int      `json:"channelCount"`
	ReserveCount    int      `json:"reserveCount"`
	UniqueTagCount  int      `json:"uniqueTagCount"`
	DuplicateCount  int      `json:"duplicateCount"`
	Warnings        []string `json:"warnings"`
}

type Group struct {
	Key            string   `json:"key"`
	ControllerName string   `json:"fcs"` // fcs сохраняется для совместимости JSON preview.
	Prefix         string   `json:"prefix"`
	POUName        string   `json:"pouName"`
	Modules        []Module `json:"modules"`
}

type Module struct {
	Name               string    `json:"name"`
	MainModule         string    `json:"mainModule"`
	RedundantModule    string    `json:"redundantModule"`
	IOType             string    `json:"ioType"`
	ObjectType         string    `json:"objectType"`
	MarshallingCabinet string    `json:"marshallingCabinet"`
	SourceRow          int       `json:"sourceRow"`
	Channels           []Channel `json:"channels"`
}

type Channel struct {
	Channel     int    `json:"channel"`
	SourceRow   int    `json:"sourceRow"`
	Tag         string `json:"tag"`
	Reserve     bool   `json:"reserve"`
	Min         string `json:"min"`
	Max         string `json:"max"`
	Duplicate   bool   `json:"duplicate"`
	DuplicateOf string `json:"duplicateOf,omitempty"`
}
