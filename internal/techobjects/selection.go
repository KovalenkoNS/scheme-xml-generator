// Выбор и переименование ПЛК для пакета технологических объектов.
package techobjects

import (
	"fmt"
	"scheme-xml-generator/internal/iomap"

	"strings"
)

// Prepare подготавливает технологические объекты выбранных ПЛК из исходного IO.
// Проверяет выбор, уникальные имена, ресурс и предел модулей; возвращает отдельный Plan на ПЛК.
func Prepare(source *iomap.Plan, selected []iomap.Selection, resource int) ([]Plan, error) {
	if source == nil || len(source.Controllers) == 0 || len(source.Controllers) > 128 || len(selected) == 0 || len(selected) > 128 {
		return nil, fmt.Errorf("технологические объекты: выберите от 1 до 128 ПЛК")
	}
	if resource < 1 || resource > 2147483647 {
		return nil, fmt.Errorf("номер ресурса должен быть целым числом от 1 до 2147483647")
	}
	available := make(map[string]iomap.Controller)
	for _, controller := range source.Controllers {
		if _, exists := available[controller.Key]; exists || controller.Key == "" {
			return nil, fmt.Errorf("повторный или пустой ключ ПЛК")
		}
		available[controller.Key] = controller
	}
	keys, names := map[string]bool{}, map[string]bool{}
	plans := make([]Plan, 0, len(selected))
	total := 0
	for _, selection := range selected {
		controller, exists := available[selection.Key]
		if !exists || keys[selection.Key] {
			return nil, fmt.Errorf("неизвестный или повторный ПЛК %q", selection.Key)
		}
		keys[selection.Key] = true
		name := strings.TrimSpace(selection.Name)
		if name == "" {
			name = controller.Name
		}
		if !plcName.MatchString(name) || names[strings.ToUpper(name)] {
			return nil, fmt.Errorf("неверное или повторное имя ПЛК %q: используйте до 100 латинских букв, цифр и знаков _", name)
		}
		names[strings.ToUpper(name)] = true
		plan, err := prepareController(controller, name, resource)
		if err != nil {
			return nil, err
		}
		total += plan.Summary.ModuleCount
		if total > 4096 {
			return nil, fmt.Errorf("допускается не более 4096 модулей за одну генерацию")
		}
		plans = append(plans, plan)
	}
	return plans, nil
}
