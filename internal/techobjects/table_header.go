// Точные заголовки и служебные ключи импортного XLS SCADA.
package techobjects

import (
	"scheme-xml-generator/internal/xls"
)

// headerRows формирует четыре строки заголовка импортной таблицы технологических объектов.
// Возвращает группы, подписи и точные служебные ключи SCADA в том же порядке столбцов.
func headerRows() [][]xls.Cell {
	const general = "ОБЩЕЕ СВ-ВО"
	const service = "Cлужебные" // Latin C matches the supplied native export.
	groups := []string{"[]D32_Вых", "[]D32_Отекстовка групп", "[]AI_DIAG_Отекстовка групп"}
	first, second := make([]string, 23), make([]string, 23)
	first[0], first[2] = service, general
	second[0], second[1] = service, service
	for i := 2; i < 20; i++ {
		second[i] = general
	}
	copy(first[20:], groups)
	copy(second[20:], groups)
	labels := []string{"№", "Статус", "Раздел", "Марка", "Тип объекта", "Наименование", "Описание", "Подпись", "Номер", "PLC переменная", "Период архив", "KKS", "Доп.параметр", "Маска упр. в срезах", "Классификатор", "Группа событий", "КОНТРОЛЛЕР", "Адрес", "№ ресурса или группа", "Шаблон", "События", "События", "События"}
	keys := []string{"-", "-", "Mode", "MARKA", "OBJTYPE", "NAME", "DISC", "OBJSIGN", "OBJNUMBER", "PLC_VARNAME", "ARH_PER", "KKS", "OBJDPARAM", "SREZCONTROL", "USERGROUP", "EVGROUP", "PLCNAME", "PLC_ADRESS", "PLC_GR", "TEMPLATE", "TEXTING", "TEXTING", "TEXTING"}
	return [][]xls.Cell{textRow(first), textRow(second), textRow(labels), textRow(keys)}
}
