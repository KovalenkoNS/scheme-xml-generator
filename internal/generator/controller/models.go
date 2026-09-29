// Controller capability rules use only explicitly confirmed CPU models.
package controller

import (
	"fmt"
	"strings"
)

const ControllerCPU715 = "TENIX-CPU715"

const ControllerCPU850 = "TENIX-CPU850"

// NormalizeSupportedControllerType Проверяет модель назначения и возвращает канонический CPU715/CPU850.
// Генераторы используют результат для выбора только подтверждённых аппаратных профилей.
func NormalizeSupportedControllerType(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value != ControllerCPU715 && value != ControllerCPU850 {
		return "", fmt.Errorf("тип контроллера должен быть %s или %s", ControllerCPU715, ControllerCPU850)
	}
	return value, nil
}
