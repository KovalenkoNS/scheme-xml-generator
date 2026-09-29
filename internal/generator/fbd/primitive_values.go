// Single-template FBD primitive values stage: library-backed output preparation and assembly.
package fbd

import (
	"fmt"

	"strconv"
	"strings"
)

// cleanNumber Нормализует числовое текстовое поле библиотечного примитива при экспорте FBD.
// Для пустого значения использует явно переданный запасной параметр формата.
func cleanNumber(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	if _, err := strconv.Atoi(value); err != nil {
		return fallback
	}
	return value
}

// normalizeBool Преобразует допустимое булево значение библиотеки к лексике SCADA XML.
// Неизвестное значение возвращает как ошибку, без неявной догадки.
func normalizeBool(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "-1":
		return "true", nil
	case "", "false", "0":
		return "false", nil
	default:
		return "", fmt.Errorf("неизвестное булево значение %q", value)
	}
}
