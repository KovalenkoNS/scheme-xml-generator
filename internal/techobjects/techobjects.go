// Package techobjects exports SCADA technological objects as a native XLS table.
// It does not create XML or allocate transport IDs.
package techobjects

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"scheme-xml-generator/internal/iomap"
	"scheme-xml-generator/internal/xls"
)

var plcName = regexp.MustCompile(`^[A-Za-z0-9_]{1,100}$`)
var moduleName = regexp.MustCompile(`^(A[0-9]{1,6})_([0-9]{2})$`)

type Summary struct {
	ModuleCount  int `json:"moduleCount"`
	ReserveCount int `json:"reserveCount"`
	ObjectCount  int `json:"objectCount"`
}

type TypeCounts struct {
	AI int `json:"ai"`
	AO int `json:"ao"`
	DI int `json:"di"`
	DO int `json:"do"`
}

type ControllerPreview struct {
	Key       string     `json:"key"`
	Name      string     `json:"name"`
	SourceFCS string     `json:"sourceFcs"`
	Cabinet   string     `json:"cabinet"`
	Types     TypeCounts `json:"types"`
	Summary
}

type Object struct {
	Tag, Type, Name, Sign, Template string
	Texting                         [3]string
}

type Plan struct {
	FCS      string
	Resource int
	Objects  []Object
	Summary  Summary
}

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
		item := ControllerPreview{Key: controller.Key, Name: controller.Name, SourceFCS: controller.SourceFCS, Cabinet: controller.Cabinet, Summary: plan.Summary}
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

func prepareController(controller iomap.Controller, name string, resource int) (Plan, error) {
	if !plcName.MatchString(name) || len(controller.Modules) == 0 || len(controller.Modules) > 4096 {
		return Plan{}, fmt.Errorf("неверное имя ПЛК или список модулей %q", name)
	}
	modules := append([]iomap.Module(nil), controller.Modules...)
	seen := make(map[string]bool)
	for _, module := range modules {
		capacity := map[string]int{"AI16H": 16, "AOC4H": 4, "DI32": 32, "DO32P": 32}[module.Type]
		parts := moduleName.FindStringSubmatch(module.Name)
		if capacity == 0 || module.Capacity != capacity || len(module.Channels) != capacity || parts == nil || module.Slot < 0 || module.Slot > 15 || module.Name != fmt.Sprintf("%s_%02d", module.Rack, module.Slot) || seen[module.Name] {
			return Plan{}, fmt.Errorf("%s: неверный или повторный модуль %s", name, module.Name)
		}
		seen[module.Name] = true
		for i, channel := range module.Channels {
			if channel.Channel != i {
				return Plan{}, fmt.Errorf("%s/%s: неверная последовательность каналов", name, module.Name)
			}
		}
	}
	sort.Slice(modules, func(i, j int) bool {
		a, b := modules[i], modules[j]
		if a.Rack != b.Rack {
			left, _ := strconv.Atoi(strings.TrimPrefix(a.Rack, "A"))
			right, _ := strconv.Atoi(strings.TrimPrefix(b.Rack, "A"))
			if left != right {
				return left < right
			}
			return a.Rack < b.Rack
		}
		return a.Slot < b.Slot
	})
	plan := Plan{FCS: name, Resource: resource, Summary: Summary{ModuleCount: len(modules)}}
	for _, module := range modules {
		plan.Objects = append(plan.Objects, diagnosticObject(name, module))
	}
	for _, module := range modules {
		objectType := map[string]string{"AI16H": "AD3_v2", "AOC4H": "AN_v1"}[module.Type]
		if objectType == "" {
			continue // DI/DO spare channels belong to the module's D32V object.
		}
		template := objectType
		if objectType == "AN_v1" {
			template = "AN"
		}
		for _, channel := range module.Channels {
			if channel.Reserve {
				tag := fmt.Sprintf("_%s_%s_%d", name, module.Name, channel.Channel)
				plan.Objects = append(plan.Objects, Object{Tag: tag, Type: objectType, Name: tag, Template: template})
				plan.Summary.ReserveCount++
			}
		}
	}
	plan.Summary.ObjectCount = len(plan.Objects)
	if plan.Summary.ObjectCount > 65532 { // Four native header rows precede objects.
		return Plan{}, fmt.Errorf("ПЛК %s: объекты не помещаются на один лист XLS (максимум 65532)", name)
	}
	return plan, nil
}

func diagnosticObject(fcs string, module iomap.Module) Object {
	tag := "_" + fcs + "_" + module.Name
	object := Object{Tag: tag, Sign: module.Name}
	label := strings.ReplaceAll(module.Name, "_", "-")
	if module.Type == "AI16H" || module.Type == "AOC4H" {
		object.Tag += "_" + module.Type
		object.Type, object.Template = "AI_DIAG16_AD3v1_kvit", "AIDIAG16_kvit"
		object.Name = "Диагностика AI/AO " + module.Type
		masks := []int{16, 64, 2048, 4096}
		if module.Type == "AOC4H" {
			masks = masks[:1]
			label = aoGroupLabel(module)
		}
		var lines []string
		for i, mask := range masks {
			lines = append(lines, eventText(int32(mask), fmt.Sprintf("%s.%d", label, i)))
		}
		object.Texting[2] = strings.Join(lines, "\n")
		return object
	}
	object.Type, object.Template = "D32V", "D32V"
	object.Name = "_" + fcs + "_IO_" + module.Type + "_" + module.Name
	var channels, groups []string
	for _, channel := range module.Channels {
		text := fmt.Sprintf("%s.i%d", tag, channel.Channel)
		if !channel.Reserve {
			text = strings.TrimSpace(channel.SourceTag + " " + channel.Description)
			if text == "" {
				text = channel.Tag
			}
			if text == "" {
				text = fmt.Sprintf("%s.i%d", tag, channel.Channel)
			}
			text += " (0->1)"
		}
		channels = append(channels, eventText(int32(uint32(1)<<channel.Channel), text))
	}
	for group := 0; group < 4; group++ {
		text := "Резерв"
		for _, channel := range module.Channels[group*8 : group*8+8] {
			if !channel.Reserve {
				text = fmt.Sprintf("%s.%d", label, group)
				break
			}
		}
		groups = append(groups, eventText(int32(uint32(1)<<(group*8)), text))
	}
	object.Texting[0], object.Texting[1] = strings.Join(channels, "\n"), strings.Join(groups, "\n")
	return object
}

// The native AO group caption joins the actual main/redundant placement,
// e.g. A11-00(01). Never infer a neighbouring module merely from its slot.
func aoGroupLabel(module iomap.Module) string {
	names := map[string]bool{module.Name: true}
	for _, channel := range module.Channels {
		if moduleName.MatchString(channel.PeerModule) {
			names[channel.PeerModule] = true
		}
	}
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	if len(ordered) == 2 && strings.Split(ordered[0], "_")[0] == strings.Split(ordered[1], "_")[0] {
		return strings.ReplaceAll(ordered[0], "_", "-") + "(" + strings.Split(ordered[1], "_")[1] + ")"
	}
	return strings.ReplaceAll(module.Name, "_", "-")
}

func eventText(mask int32, label string) string {
	// Colons and newlines are delimiters of the SCADA event table, not escaped
	// text. Keep source descriptions readable without creating extra entries.
	label = strings.Join(strings.Fields(strings.ReplaceAll(label, ":", " — ")), " ")
	return fmt.Sprintf("n%d:%s:не вывод::не вывод", mask, label)
}

func Generate(plan Plan) ([]byte, error) {
	if !plcName.MatchString(plan.FCS) || plan.Resource < 1 || plan.Resource > 2147483647 || len(plan.Objects) == 0 || len(plan.Objects) > 65532 || plan.Summary.ObjectCount != len(plan.Objects) || plan.Summary.ModuleCount+plan.Summary.ReserveCount != len(plan.Objects) {
		return nil, fmt.Errorf("неверный план технологических объектов")
	}
	rows := headerRows()
	seen := map[string]bool{}
	for _, object := range plan.Objects {
		if !strings.HasPrefix(object.Tag, "_"+plan.FCS+"_") || object.Type == "" || object.Template == "" || seen[strings.ToUpper(object.Tag)] {
			return nil, fmt.Errorf("повторный или неверный объект %q", object.Tag)
		}
		seen[strings.ToUpper(object.Tag)] = true
		values := []string{"", "", "TEHOBJ", object.Tag, object.Type, object.Name, "", object.Sign, "0", "", "0", "", "", "1", "[ВСЕ]", `[Все]\[Все технологические]`, plan.FCS, "0", strconv.Itoa(plan.Resource), object.Template, object.Texting[0], object.Texting[1], object.Texting[2]}
		rows = append(rows, textRow(values))
	}
	format, err := nativeFormatting()
	if err != nil {
		return nil, err
	}
	return xls.WriteWithFormatting("Sheet1", rows, format)
}

func textRow(values []string) []xls.Cell {
	// Preserve the native export's 100 columns, including the unused final
	// heading band and styled blank cells. Its 23 populated columns stay fixed.
	row := make([]xls.Cell, 100)
	for i, value := range values {
		if value != "" {
			row[i] = xls.Text(value)
		}
	}
	return row
}

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
