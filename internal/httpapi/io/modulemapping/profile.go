// Метаданные подтверждённого профиля и доступности сценариев.
package modulemapping

import (
	"net/http"
	cpuprofile "scheme-xml-generator/internal/domain/controller"
	"scheme-xml-generator/internal/generator/addressing"
	programcontext "scheme-xml-generator/internal/generator/program"
	"scheme-xml-generator/internal/httpapi/transport"
)

// HandleModuleMappingProfile выдаёт параметры совместимого HTTP-сценария локальных IO-перекладок.
// Возвращает профили по формату/типу сигнала и честные ограничения исторических образцов.
func (s *Service) HandleModuleMappingProfile(w http.ResponseWriter, _ *http.Request) {
	transport.WriteJSON(w, http.StatusOK, map[string]any{
		"context":          addressing.DefaultModuleContext(),
		"fbdAvailable":     false,
		"fbdRoute":         "/api/generate",
		"contexts":         map[string]programcontext.ProgramContext{"AI": ContextForMode("AI", "st"), "DO": ContextForMode("DO", "st"), "DI": ContextForMode("DI", "st")},
		"doFBDProfiles":    []string{},
		"controllerTypes":  []string{cpuprofile.ControllerCPU715, cpuprofile.ControllerCPU850},
		"physicalProfiles": []string{addressing.PhysicalProfileLegacy, addressing.PhysicalProfileMeasurement},
		"defaultPhysicalProfiles": map[string]string{
			cpuprofile.ControllerCPU715: addressing.PhysicalProfileLegacy,
			cpuprofile.ControllerCPU850: addressing.PhysicalProfileMeasurement,
		},
		"description": "ST назначает физические каналы выбранных ПЛК. ModuleID задаются явно. FBD создаётся отдельным библиотечным запросом; прежние фиксированные режимы отключены.",
	})
}

// ContextForMode выбирает исходные транспортные значения подтверждённых ST-образцов по направлению.
// Возвращает копию контекста с нужными ID/POUNum; не меняет настройки основного генератора.
func ContextForMode(kind, mode string) programcontext.ProgramContext {
	ctx := addressing.DefaultModuleContext()
	if kind == "DI" {
		ctx.ControllerID, ctx.ResourceID, ctx.GroupID = "189312", "644", "19814"
		ctx.POUNumber = "23"
	} else if kind == "DO" {
		ctx.GroupID, ctx.POUNumber = "19913", "47"
	}
	return ctx
}
