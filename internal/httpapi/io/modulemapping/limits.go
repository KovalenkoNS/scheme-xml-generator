// Пределы входного плана до выделения идентификаторов.
package modulemapping

import (
	"fmt"

	"scheme-xml-generator/internal/domain/assignments"
	"scheme-xml-generator/internal/httpapi/limits"
)

// CheckModuleMappingSize проверяет исходный план IO до подготовки дополнительных физических каналов.
// Сверяет группы, модули и исходные сигналы с общими HTTP-лимитами; итоговый ST-план проверяется отдельно.
func CheckModuleMappingSize(plan *assignments.Plan) error {
	if plan == nil || len(plan.Groups) == 0 || len(plan.Groups) > limits.MaxDocumentPOUs {
		return fmt.Errorf("назначения модулей должны содержать от 1 до %d групп AI/DO/DI", limits.MaxDocumentPOUs)
	}
	modules, signals := 0, 0
	for _, group := range plan.Groups {
		modules += len(group.Modules)
		for _, module := range group.Modules {
			signals += len(module.Channels)
		}
	}
	if modules > limits.MaxDocumentModules || signals > limits.MaxDocumentSignals {
		return fmt.Errorf("назначения превышают пределы %d модулей / %d каналов", limits.MaxDocumentModules, limits.MaxDocumentSignals)
	}
	return nil
}
