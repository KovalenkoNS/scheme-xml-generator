// Подтверждённые аппаратные модули карт назначений и их программные получатели ST.
// Таблица не является каталогом библиотечных FBD-шаблонов и не выбирает XML-диалект CPU.
package moduleprofile

import "scheme-xml-generator/internal/domain/hardware"

// Definition связывает направление IO с проверенным модулем и моделью назначений.
// Планировщик использует эти свойства, не закрепляя аппаратные имена в своём обходе.
type Definition struct {
	Kind                  string
	Capacity              int
	HardwareType          string
	ObjectType            string
	AllowEmpty            bool
	AllPhysicalSTChannels bool
	AssignmentsPerSignal  int
	AssignmentsPerModule  int
	ReceiverFields        []string
}

// Lookup возвращает только профили, подтверждённые существующими картами и ST-образцами.
// AO имеет отдельный контракт карты; неизвестное направление здесь не подменяется AI.
func Lookup(kind string) (Definition, bool) {
	var profile Definition
	switch kind {
	case "AI":
		profile = Definition{Kind: kind, HardwareType: "AI16H", ObjectType: "AD3_v2", AssignmentsPerSignal: 2}
	case "DO":
		profile = Definition{Kind: kind, HardwareType: "DO32P", ObjectType: "D32V", AllPhysicalSTChannels: true, AssignmentsPerSignal: 1}
	case "DI":
		profile = Definition{Kind: kind, HardwareType: "DI32", ObjectType: "D32V", AllowEmpty: true, AllPhysicalSTChannels: true, ReceiverFields: []string{"C1", "C2"}}
	default:
		return Definition{}, false
	}
	module, ok := hardware.Lookup(profile.HardwareType)
	profile.Capacity = module.Channels
	if profile.AllPhysicalSTChannels {
		profile.AssignmentsPerModule = module.Channels
	}
	if kind == "DI" {
		profile.AssignmentsPerModule++
	}
	return profile, ok
}

// AssignmentCounts сохраняет подсчёт существующей сводки модульного экспорта.
// DI считает статус и все входы модуля; DO ST считает все выходы, AI — пару назначений сигнала.
func (d Definition) AssignmentCounts(mode string) (perModule, perSignal int) {
	if d.Kind == "DI" || d.Kind == "DO" && mode == "st" {
		return d.AssignmentsPerModule, 0
	}
	return 0, d.AssignmentsPerSignal
}

// AcceptsReceiver проверяет поле получателя, заданное подтверждённой моделью модуля.
// Пустой список полей означает, что ограничение C1/C2 к этому направлению не относится.
func (d Definition) AcceptsReceiver(member string) bool {
	if len(d.ReceiverFields) == 0 {
		return true
	}
	for _, field := range d.ReceiverFields {
		if member == field {
			return true
		}
	}
	return false
}
