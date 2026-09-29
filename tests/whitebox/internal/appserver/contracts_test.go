// Test-only aliases keep HTTP assertions on real response contracts after subject relocation.
package appserver

import (
	ioao "scheme-xml-generator/internal/httpapi/io/ao"
	ioupload "scheme-xml-generator/internal/httpapi/io/upload"
	"scheme-xml-generator/internal/httpapi/limits"
	"scheme-xml-generator/internal/httpapi/output"
	"scheme-xml-generator/internal/httpapi/transport"
)

type generateResponse = output.GenerateResponse
type aoGenerateResponse = output.BatchResponse
type aoGeneratedFile = output.GeneratedControllerFile
type aoBatchSummary = output.BatchSummary
type techObjectsResponse = output.TechObjectsResponse
type techObjectsGeneratedFile = output.TechObjectsGeneratedFile

const (
	maxDocumentPOUs       = limits.MaxDocumentPOUs
	maxDocumentSignals    = limits.MaxDocumentSignals
	maxDocumentModules    = limits.MaxDocumentModules
	maxDocumentObjects    = limits.MaxDocumentObjects
	maxDocumentCards      = limits.MaxDocumentCards
	maxAOMappingFileBytes = ioao.MaxAOMappingFileBytes
	maxAOMultipartExtra   = ioupload.MaxAOMultipartExtra
)

var decodeJSON = transport.DecodeJSON

type techObjectsBatchItem = output.TechObjectsBatchItem

// outputFile описывает JSON-контракт списка файлов, а не копирует реализацию хранилища.
type outputFile struct {
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	CreatedAt string `json:"createdAt"`
	URL       string `json:"url"`
}
