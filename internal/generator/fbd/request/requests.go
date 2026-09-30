// Библиотечные FBD-запросы и привязанные снимки шаблонов; без алгоритмов ST/HMI и состояния allocator.
package request

import (
	"scheme-xml-generator/internal/library"
)

type Request struct {
	TemplateKey string             `json:"templateKey"`
	ObjectName  string             `json:"objectName"`
	POUName     string             `json:"pouName"`
	NameMode    string             `json:"nameMode"`
	Description string             `json:"description"`
	ClusterPath string             `json:"klPath"`
	OffsetX     *int               `json:"offsetX"`
	OffsetY     *int               `json:"offsetY"`
	FileName    string             `json:"fileName"`
	T11Start    *int64             `json:"t11Start"`
	CardStart   *int64             `json:"cardStart"`
	POUID       *int64             `json:"pouId"`
	POUGroupID  *int64             `json:"pouGroupId"`
	POUNumber   *int64             `json:"pouNumber"`
	POUs        []POURequest       `json:"pous,omitempty"`
	Context     *GenerationContext `json:"context,omitempty"`
}

// POURequest describes one independently configured POU in a document.
// Pointer fields in Page preserve the distinction between an omitted value
// (use the application defaults) and an explicitly supplied zero.
type POURequest struct {
	Name               string          `json:"name"`
	Description        string          `json:"description"`
	POUID              *int64          `json:"pouId,omitempty"`
	GroupID            *int64          `json:"groupId,omitempty"`
	POUNumber          *int64          `json:"pouNumber,omitempty"`
	Page               PageRequest     `json:"page"`
	DefaultTemplateKey string          `json:"defaultTemplateKey,omitempty"`
	IO                 *IORequest      `json:"io,omitempty"`
	Signals            []SignalRequest `json:"signals"`
}

// IORequest retains the retired physical-IO wire shape solely for an explicit rejection.
// Any non-nil IO section fails in both HTTP and core; library DO uses its own validated request.
type IORequest struct {
	Type    string            `json:"type"`
	Modules []IOModuleRequest `json:"modules"`
}

type IOModuleRequest struct {
	ID            *int64          `json:"id,omitempty"`
	BindingPrefix string          `json:"bindingPrefix,omitempty"`
	InstanceName  string          `json:"instanceName,omitempty"`
	Signals       []SignalRequest `json:"signals"`
}

type PageRequest struct {
	Width           *int    `json:"width,omitempty"`
	Height          *int    `json:"height,omitempty"`
	DParams         *string `json:"dparams,omitempty"`
	BackgroundColor *string `json:"backgroundColor,omitempty"`
	MarginRight     *int    `json:"marginRight,omitempty"`
	MarginBottom    *int    `json:"marginBottom,omitempty"`
}

// SignalRequest contains only instance-level settings. POU and document
// settings deliberately live at their respective parent levels.
type SignalRequest struct {
	TemplateKey string `json:"templateKey"`
	ObjectName  string `json:"objectName"`
	NameMode    string `json:"nameMode"`
	Description string `json:"description"`
	ClusterPath string `json:"klPath"`
	OffsetX     *int   `json:"offsetX,omitempty"`
	OffsetY     *int   `json:"offsetY,omitempty"`
	Invert      bool   `json:"invert,omitempty"`
}

// ResolvedSignal pins a request to the exact in-memory library snapshot from
// which it will be generated. The FBD HTTP boundary resolves keys before reserving IDs.
type ResolvedSignal struct {
	Request SignalRequest
	Ref     *library.TemplateRef
}

type ResolvedPOU struct {
	Request POURequest
	Signals []ResolvedSignal
}

type NamePreview struct {
	BaseName      string   `json:"baseName"`
	MatchedPrefix string   `json:"matchedPrefix"`
	ObjectNames   []string `json:"objectNames"`
}

// GenerationContext overrides the destination of one library FBD request.
// An omitted context preserves config.json, including its portable defaults.
// CPU selection never rewrites the application's configuration.
type GenerationContext struct {
	ControllerTypeName string  `json:"controllerTypeName"`
	Version            *string `json:"version,omitempty"`
	Project            *string `json:"project,omitempty"`
	ControllerID       *string `json:"controllerId,omitempty"`
	ResourceID         *string `json:"resourceId,omitempty"`
}

const DefaultSignalOffsetX = 300

const DefaultSignalOffsetY = 100

const DefaultSignalGapY = 20

const MaxPageExtent = 1000000
