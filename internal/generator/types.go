package generator

import (
	"scheme-xml-generator/internal/config"
	"scheme-xml-generator/internal/library"
)

type Request struct {
	TemplateKey string       `json:"templateKey"`
	ObjectName  string       `json:"objectName"`
	POUName     string       `json:"pouName"`
	NameMode    string       `json:"nameMode"`
	Description string       `json:"description"`
	ClusterPath string       `json:"klPath"`
	OffsetX     *int         `json:"offsetX"`
	OffsetY     *int         `json:"offsetY"`
	FileName    string       `json:"fileName"`
	T11Start    *int64       `json:"t11Start"`
	CardStart   *int64       `json:"cardStart"`
	POUID       *int64       `json:"pouId"`
	POUGroupID  *int64       `json:"pouGroupId"`
	POUNumber   *int64       `json:"pouNumber"`
	POUs        []POURequest `json:"pous,omitempty"`
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

// IORequest describes the physical I/O layer of one POU. Signals are grouped
// by hardware module; their channel is their zero-based position in the
// module. A POU uses either this form or the legacy flat Signals form.
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
}

// ResolvedSignal pins a request to the exact in-memory library snapshot from
// which it will be generated. The appserver resolves keys before reserving IDs.
type ResolvedSignal struct {
	Request SignalRequest
	Ref     *library.TemplateRef
}

type ResolvedPOU struct {
	Request POURequest
	Signals []ResolvedSignal
}

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

type DiagnosticFrameSummary struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Module string `json:"module"`
}

type NamePreview struct {
	BaseName      string   `json:"baseName"`
	MatchedPrefix string   `json:"matchedPrefix"`
	ObjectNames   []string `json:"objectNames"`
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

type Generator struct {
	Config config.Config
}
