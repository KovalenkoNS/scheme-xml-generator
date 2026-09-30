// Планы физических ST-назначений выбранного ПЛК и сводка их выпуска; исходные форматы не входят в модель.
package assignment

import (
	"scheme-xml-generator/internal/domain/assignments"
	xmlartifact "scheme-xml-generator/internal/generator/artifact"
	xmlidentity "scheme-xml-generator/internal/generator/identity"
	programcontext "scheme-xml-generator/internal/generator/program"
)

// ModuleMappingRequest выбирает физические ST-назначения и явные ModuleID в порядке предварительного просмотра.
// DOFBDProfile сохранён только для распознавания и отказа устаревшего входного контракта.
type ModuleMappingRequest struct {
	Kind         string               `json:"kind"`
	DOFBDProfile string               `json:"doFBDProfile,omitempty"`
	POUs         []ModuleGroupRequest `json:"pous"`
}

type ModuleGroupRequest struct {
	GroupKey    string   `json:"groupKey"`
	ModuleCount *int     `json:"moduleCount,omitempty"`
	ModuleIDs   []*int64 `json:"moduleIds"`
}

type ControllerPlan struct {
	ControllerName          string
	Kind                    string
	POUs                    []ModuleGroup
	ModuleCount             int
	SignalCount             int
	AssignmentCount         int
	RepeatedAssignmentCount int
	Warnings                []string
}

type ModuleGroup struct {
	GroupKey string
	Name     string
	Kind     string
	Prefix   string
	Modules  []PhysicalModule
}

type PhysicalModule struct {
	Name       string
	Type       string
	ObjectType string
	Capacity   int
	ID         *int64
	Channels   []assignments.Channel
}

// ControllerSummary Собирает сводку выбранных модулей и диапазонов ID для ответа генератора модулей.
// Использует подготовленный план и контекст ПЛК, не меняет XML или состояние allocator.
func ControllerSummary(plan ControllerPlan, ctx programcontext.ProgramContext, ids xmlidentity.IDRange) xmlartifact.Summary {
	return xmlartifact.Summary{POUCount: len(plan.POUs), IOModuleCount: plan.ModuleCount, SignalCount: plan.SignalCount, POUID: ids.POUID, POUName: plan.POUs[0].Name, POUGroupID: ctx.GroupID, POUNumber: ctx.POUNumber, POUs: []xmlartifact.POUSummary{}}
}
