// Проверка границы входных форматов: пара Xin/Xs требуется только подготовленной карте, не всей модели AI.
package assignments_test

import (
	model "scheme-xml-generator/internal/domain/assignments"
	"scheme-xml-generator/internal/inputs/assignments/assembly"
	"scheme-xml-generator/internal/inputs/assignments/prepared"
	"strings"
	"testing"
)

// TestPreparedPairValidationRespectsSourceFormat защищает независимость общей модели AI от синтаксиса Excel-карты.
// Та же неполная маска допустима для другого адаптера, но помеченный prepared-вход обязан содержать Xin и Xs.
func TestPreparedPairValidationRespectsSourceFormat(t *testing.T) {
	module := &assembly.Module{
		Group:    model.Group{Kind: "AI", ControllerName: "plc"},
		Module:   model.Module{Name: "A11-02"},
		Channels: map[int]*assembly.Channel{0: {Channel: model.Channel{Channel: 0, SourceRow: 7}, Parts: 1}},
	}
	modules := map[string]*assembly.Module{"PLC:A11-02": module}
	names := map[string]string{"PLC": "PLC"}
	if err := prepared.ValidateComplete(modules, names); err != nil {
		t.Fatalf("another adapter inherited prepared-expression rules: %v", err)
	}
	module.PreparedAIPair = true
	err := prepared.ValidateComplete(modules, names)
	if err == nil || !strings.Contains(err.Error(), "PLC/A11-02") || !strings.Contains(err.Error(), "Xin и Xs") {
		t.Fatalf("incomplete prepared pair must retain source context: %v", err)
	}
	module.Channels[0].Parts = 3
	if err := prepared.ValidateComplete(modules, names); err != nil {
		t.Fatalf("complete prepared pair was rejected: %v", err)
	}
}
