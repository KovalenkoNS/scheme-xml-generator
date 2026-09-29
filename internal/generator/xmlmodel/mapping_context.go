// Общий XML-контекст модулей переносит нормализованное назначение импорта в заголовок FBD или ST.
package xmlmodel

import (
	"scheme-xml-generator/internal/generator/contracts"
)

// MappingCommon Переносит константы контекста перекладок в общую секцию выходного XML.
// Возвращает транспортную модель Common для генераторов ST/FBD.
func MappingCommon(ctx contracts.ProgramContext) OutputCommon {
	return OutputCommon{Version: ctx.Version, Project: ctx.Project, IsCut: "false", IsFFB: "false", ControllerType: ctx.ControllerTypeName, ControllerID: ctx.ControllerID, ResourceID: ctx.ResourceID}
}
