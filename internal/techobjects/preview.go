// Список ПЛК и счётчики технологических объектов для выбора до генерации XLS.
package techobjects

import (
	"fmt"
	"scheme-xml-generator/internal/iomap"
)

// Preview строит список ПЛК и количество технологических объектов для выбора в интерфейсе.
// Получает IO-план, проверяет каждый контроллер и возвращает счётчики модулей/резервов без XLS.
func Preview(source *iomap.Plan) ([]ControllerPreview, error) {
	if source == nil || len(source.Controllers) == 0 || len(source.Controllers) > 128 {
		return nil, fmt.Errorf("технологические объекты: в IO нет допустимого списка ПЛК")
	}
	result := make([]ControllerPreview, 0, len(source.Controllers))
	for _, controller := range source.Controllers {
		plan, err := prepareController(controller, controller.Name, 1)
		if err != nil {
			return nil, err
		}
		item := ControllerPreview{Key: controller.Key, Name: controller.Name, SourceController: controller.SourceController, Cabinet: controller.Cabinet, Summary: plan.Summary}
		for _, module := range controller.Modules {
			switch module.Type {
			case "AI16H":
				item.Types.AI++
			case "AOC4H":
				item.Types.AO++
			case "DI32":
				item.Types.DI++
			case "DO32P":
				item.Types.DO++
			}
		}
		result = append(result, item)
	}
	return result, nil
}
