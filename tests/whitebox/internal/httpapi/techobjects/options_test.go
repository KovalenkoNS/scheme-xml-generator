// Проверка локального значения по умолчанию до генерации XLS технологических объектов.
package techobjectapi

import "testing"

// TestDefaultResource проверяет прежнее утверждение, перенесённое из HTTP-сценария.
// Декодирует выбранный ПЛК без resourceNumber и ожидает фактическое значение ресурса 1.
func TestDefaultResource(t *testing.T) {
	parsed, err := decodeTechObjectsOptions(`{"controllers":[{"key":"FCS8:3000_D_SC_B07"}]}`)
	if err != nil || parsed.ResourceNumber != 1 {
		t.Fatalf("default resource: %+v, %v", parsed, err)
	}
}
