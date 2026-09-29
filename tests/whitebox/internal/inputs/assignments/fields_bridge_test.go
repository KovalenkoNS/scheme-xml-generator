// Test-only bridge keeps historical module normalisation checks in the separate suite.
package assignments

import (
	"scheme-xml-generator/internal/inputs/assignments/fields"
	"scheme-xml-generator/internal/inputs/assignments/raw"
	"scheme-xml-generator/internal/iomap"
)

// normalizeModule exercises the actual format adapter without exposing a production test API.
func normalizeModule(value string) (string, string, int, error) { return fields.NormalizeModule(value) }

// readRawIOHeader checks the source-format header using its actual adapter.
func readRawIOHeader(row iomap.Row) (map[string]string, bool, error) { return raw.Header(row) }
