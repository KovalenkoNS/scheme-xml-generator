// Готовый XML и сводки результата генерации; не HTTP-запрос и не изменяемая модель оборудования.
package artifact

type DiagnosticFrameSummary struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Module string `json:"module"`
}

type Summary struct {
	FrameCount    int                      `json:"frameCount,omitempty"`
	Frames        []DiagnosticFrameSummary `json:"frames,omitempty"`
	Blocks        int                      `json:"blocks"`
	Links         int                      `json:"links"`
	Graphics      int                      `json:"graphics"`
	Cards         int                      `json:"cards"`
	T11First      int64                    `json:"t11First"`
	T11Last       int64                    `json:"t11Last"`
	CardFirst     int64                    `json:"cardFirst"`
	CardLast      int64                    `json:"cardLast"`
	POUID         int64                    `json:"pouId"`
	POUName       string                   `json:"pouName"`
	POUGroupID    string                   `json:"pouGroupId"`
	POUNumber     string                   `json:"pouNumber"`
	POUCount      int                      `json:"pouCount"`
	SignalCount   int                      `json:"signalCount"`
	IOModuleCount int                      `json:"ioModuleCount,omitempty"`
	POUs          []POUSummary             `json:"pous"`
}

type SignalSummary struct {
	TemplateKey string `json:"templateKey"`
	BaseName    string `json:"baseName"`
	Blocks      int    `json:"blocks"`
	Links       int    `json:"links"`
	Graphics    int    `json:"graphics"`
	Cards       int    `json:"cards"`
	T11First    int64  `json:"t11First"`
	T11Last     int64  `json:"t11Last"`
	CardFirst   int64  `json:"cardFirst"`
	CardLast    int64  `json:"cardLast"`
	OffsetX     int    `json:"offsetX"`
	OffsetY     int    `json:"offsetY"`
	IOType      string `json:"ioType,omitempty"`
	ModuleID    *int64 `json:"moduleId,omitempty"`
	Channel     *int   `json:"channel,omitempty"`
}

type IOModuleSummary struct {
	Type          string `json:"type"`
	ID            int64  `json:"id"`
	BindingPrefix string `json:"bindingPrefix"`
	InstanceName  string `json:"instanceName"`
	Capacity      int    `json:"capacity"`
	SignalCount   int    `json:"signalCount"`
	Blocks        int    `json:"blocks"`
	Links         int    `json:"links"`
	Cards         int    `json:"cards"`
	T11First      int64  `json:"t11First"`
	T11Last       int64  `json:"t11Last"`
	CardFirst     int64  `json:"cardFirst"`
	CardLast      int64  `json:"cardLast"`
}

type POUSummary struct {
	POUID      int64             `json:"pouId"`
	POUName    string            `json:"pouName"`
	POUGroupID string            `json:"pouGroupId"`
	POUNumber  string            `json:"pouNumber"`
	Blocks     int               `json:"blocks"`
	Links      int               `json:"links"`
	Graphics   int               `json:"graphics"`
	Cards      int               `json:"cards"`
	T11First   int64             `json:"t11First"`
	T11Last    int64             `json:"t11Last"`
	CardFirst  int64             `json:"cardFirst"`
	CardLast   int64             `json:"cardLast"`
	Signals    []SignalSummary   `json:"signals"`
	IOModules  []IOModuleSummary `json:"ioModules,omitempty"`
}

type Result struct {
	XML      []byte
	BaseName string
	Summary  Summary
	Warnings []string
}
