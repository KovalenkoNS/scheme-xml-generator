package techobjects

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"scheme-xml-generator/internal/xls"
)

// Native format includes the original font/XF tables, row heights, column
// width and merged heading ranges. Source files and Excel are not needed at
// runtime. The covered cells retain their hidden import metadata values.
//
//go:embed assets/xls_format.json
var nativeFormattingJSON []byte

func nativeFormatting() (xls.Formatting, error) {
	var format xls.Formatting
	if err := json.Unmarshal(nativeFormattingJSON, &format); err != nil {
		return format, fmt.Errorf("прочитать встроенное оформление XLS: %w", err)
	}
	return format, nil
}
