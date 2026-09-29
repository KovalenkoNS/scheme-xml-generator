// Метаданные подтверждённого профиля и доступности сценариев.
package modulemapping

import (
	"net/http"
	"scheme-xml-generator/internal/generator"

	"scheme-xml-generator/internal/httpapi/transport"
)

// HandleModuleMappingProfile выдаёт параметры совместимого HTTP-сценария локальных IO-перекладок.
// Возвращает профили по формату/типу сигнала и честные ограничения исторических образцов.
func (s *Service) HandleModuleMappingProfile(w http.ResponseWriter, _ *http.Request) {
	transport.WriteJSON(w, http.StatusOK, map[string]any{
		"context":          generator.DefaultModuleContext(),
		"fbdAvailable":     false,
		"fbdRoute":         "/api/generate",
		"contexts":         map[string]generator.ProgramContext{"AI": ContextForMode("AI", "st"), "DO": ContextForMode("DO", "st"), "DI": ContextForMode("DI", "st")},
		"doFBDProfiles":    []string{},
		"controllerTypes":  []string{generator.ControllerCPU715, generator.ControllerCPU850},
		"physicalProfiles": []string{generator.PhysicalProfileLegacy, generator.PhysicalProfileMeasurement},
		"defaultPhysicalProfiles": map[string]string{
			generator.ControllerCPU715: generator.PhysicalProfileLegacy,
			generator.ControllerCPU850: generator.PhysicalProfileMeasurement,
		},
		"description": "ST назначает физические каналы выбранных ПЛК. ModuleID задаются явно. FBD создаётся отдельным библиотечным запросом; прежние фиксированные режимы отключены.",
	})
}

// ContextForMode выбирает исходные транспортные значения подтверждённых ST-образцов по направлению.
// Возвращает копию контекста с нужными ID/POUNum; не меняет настройки основного генератора.
func ContextForMode(kind, mode string) generator.ProgramContext {
	ctx := generator.DefaultModuleContext()
	if kind == "DI" {
		ctx.ControllerID, ctx.ResourceID, ctx.GroupID = "189312", "644", "19814"
		ctx.POUNumber = "23"
	} else if kind == "DO" {
		ctx.GroupID, ctx.POUNumber = "19913", "47"
	}
	return ctx
}
