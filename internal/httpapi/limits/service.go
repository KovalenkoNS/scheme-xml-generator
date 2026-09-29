// Общие пределы HTTP-запросов генерации; не содержат предметной обработки.
package limits

import ()

const (
	MaxDocumentPOUs    = 128
	MaxDocumentSignals = 4096
	MaxDocumentModules = 4096
	MaxDocumentObjects = 200000
	MaxDocumentCards   = 100000
)
