// Сериализация проверенных объектов ПЛК в XLS с подтверждённым оформлением.
package techobjects

import (
	"fmt"

	"scheme-xml-generator/internal/xls"
	"strconv"
	"strings"
)

// Generate сериализует проверенный план технологических объектов в настоящий XLS.
// Проверяет теги/счётчики, формирует строки и вызывает xls.WriteWithFormatting с встроенным оформлением.
func Generate(plan Plan) ([]byte, error) {
	if !plcName.MatchString(plan.ControllerName) || plan.Resource < 1 || plan.Resource > 2147483647 || len(plan.Objects) == 0 || len(plan.Objects) > 65532 || plan.Summary.ObjectCount != len(plan.Objects) || plan.Summary.ModuleCount+plan.Summary.ReserveCount != len(plan.Objects) {
		return nil, fmt.Errorf("неверный план технологических объектов")
	}
	rows := headerRows()
	seen := map[string]bool{}
	for _, object := range plan.Objects {
		if !strings.HasPrefix(object.Tag, "_"+plan.ControllerName+"_") || object.Type == "" || object.Template == "" || seen[strings.ToUpper(object.Tag)] {
			return nil, fmt.Errorf("повторный или неверный объект %q", object.Tag)
		}
		seen[strings.ToUpper(object.Tag)] = true
		values := []string{"", "", "TEHOBJ", object.Tag, object.Type, object.Name, "", object.Sign, "0", "", "0", "", "", "1", "[ВСЕ]", `[Все]\[Все технологические]`, plan.ControllerName, "0", strconv.Itoa(plan.Resource), object.Template, object.Texting[0], object.Texting[1], object.Texting[2]}
		rows = append(rows, textRow(values))
	}
	format, err := nativeFormatting()
	if err != nil {
		return nil, err
	}
	return xls.WriteWithFormatting("Sheet1", rows, format)
}
