// Ограничения подтверждённых XML-профилей отделены от свойств CPU и областей инженерного проекта.
package exportprofile

import (
	"fmt"
	"scheme-xml-generator/internal/domain/controller"
)

// ValidateAOPhysicalST допускает только CPU с подтверждённым физическим AO ST-экспортом.
// Отказ означает отсутствие реализации профиля, а не отсутствие AO у оборудования или в области ПАЗ.
func ValidateAOPhysicalST(cpu string) error {
	if cpu != controller.ControllerCPU715 {
		return fmt.Errorf("физический AO ST-профиль для %s не подтверждён; поддерживается %s", cpu, controller.ControllerCPU715)
	}
	return nil
}

// DiagnosticObjectType возвращает подтверждённый программный тип аналогового канала для HMI.
// Цифровые кадры не налагают это аналоговое ограничение; аппаратная ёмкость принадлежит domain/hardware.
func DiagnosticObjectType(module string) string {
	switch module {
	case "AI16H":
		return "AD3_v2"
	case "AOC4H":
		return "AN_v1"
	}
	return ""
}
