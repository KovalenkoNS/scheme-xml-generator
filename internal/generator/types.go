package generator

import (
	"scheme-xml-generator/internal/config"
)

type Request struct {
	TemplateKey string `json:"templateKey"`
	ObjectName  string `json:"objectName"`
	POUName     string `json:"pouName"`
	NameMode    string `json:"nameMode"`
	Description string `json:"description"`
	ClusterPath string `json:"klPath"`
	OffsetX     *int   `json:"offsetX"`
	OffsetY     *int   `json:"offsetY"`
	FileName    string `json:"fileName"`
	T11Start    *int64 `json:"t11Start"`
	CardStart   *int64 `json:"cardStart"`
	POUID       *int64 `json:"pouId"`
	POUGroupID  *int64 `json:"pouGroupId"`
	POUNumber   *int64 `json:"pouNumber"`
}

type IDRange struct {
	T11Start  int64
	CardStart int64
	POUID     int64
}

type NamePreview struct {
	BaseName      string   `json:"baseName"`
	MatchedPrefix string   `json:"matchedPrefix"`
	ObjectNames   []string `json:"objectNames"`
}

type Summary struct {
	Blocks     int    `json:"blocks"`
	Links      int    `json:"links"`
	Graphics   int    `json:"graphics"`
	Cards      int    `json:"cards"`
	T11First   int64  `json:"t11First"`
	T11Last    int64  `json:"t11Last"`
	CardFirst  int64  `json:"cardFirst"`
	CardLast   int64  `json:"cardLast"`
	POUID      int64  `json:"pouId"`
	POUName    string `json:"pouName"`
	POUGroupID string `json:"pouGroupId"`
	POUNumber  string `json:"pouNumber"`
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
