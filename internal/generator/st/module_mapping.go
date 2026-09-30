// Генератор ST модулей формирует присваивания каналов и статусов по подтверждённому аппаратному профилю.
package st

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"reflect"
	"scheme-xml-generator/internal/generator/addressing"
	xmlartifact "scheme-xml-generator/internal/generator/artifact"
	xmlidentity "scheme-xml-generator/internal/generator/identity"
	moduleid "scheme-xml-generator/internal/generator/modules"
	programcontext "scheme-xml-generator/internal/generator/program"
	stassignment "scheme-xml-generator/internal/generator/st/assignment"
	"scheme-xml-generator/internal/generator/xmlcodec"
	"scheme-xml-generator/internal/generator/xmlmodel"
	"strconv"
	"strings"
)

// doAssignment Формирует ST-присваивание для одного DO-канала физического модуля.
// Использует выбранный профиль адресации и имя экземпляра ПЛК/модуля.
func doAssignment(controllerName string, module stassignment.PhysicalModule, channel int, profile string) string {
	if profile == addressing.PhysicalProfileLegacy {
		return fmt.Sprintf("_IO_QU%d_%d.Value := %s._%02d;", *module.ID, channel, moduleid.ModuleInstanceTag(controllerName, module.Name), channel)
	}
	return fmt.Sprintf("_IO_Q%d_DO32P_%d_VAL.Measurement := %s._%02d;", *module.ID, channel, moduleid.ModuleInstanceTag(controllerName, module.Name), channel)
}

// bindingPrefix Выбирает префикс физической переменной ST по направлению, профилю и ModuleID.
// Связывает текст перекладки с подтверждённым аппаратным форматом.
func bindingPrefix(kind, profile string, id int64) string {
	if profile == addressing.PhysicalProfileLegacy {
		if kind == "DO" {
			return fmt.Sprintf("_IO_QU%d", id)
		}
		return fmt.Sprintf("_IO_IU%d", id)
	}
	if kind == "DO" {
		return fmt.Sprintf("_IO_Q%d_DO32P", id)
	}
	if kind == "DI" {
		return fmt.Sprintf("_IO_I%d_DI32", id)
	}
	return fmt.Sprintf("_IO_I%d_AI16H", id)
}

// GenerateModuleST Создаёт ST-программы перекладок по проверенному плану AI/DI/DO.
// Использует контекст и выделенные POU ID, возвращает структурно проверенный XML и сводку.
func GenerateModuleST(plan stassignment.ControllerPlan, ctx programcontext.ProgramContext, ids xmlidentity.IDRange) (xmlartifact.Result, error) {
	doc := xmlmodel.OutputSTDocument{XMLName: xml.Name{Local: "BufScadaPOUS"}, Common: xmlmodel.MappingCommon(ctx)}
	summary := stassignment.ControllerSummary(plan, ctx, ids)
	firstNumber, _ := strconv.ParseInt(ctx.POUNumber, 10, 32)
	hasAI, hasDI := false, false
	for index, pou := range plan.POUs {
		id, number := ids.POUID+int64(index), strconv.FormatInt(firstNumber+int64(index), 10)
		ps := xmlartifact.POUSummary{POUID: id, POUName: pou.Name, POUGroupID: ctx.GroupID, POUNumber: number, Signals: []xmlartifact.SignalSummary{}}
		var code strings.Builder
		fmt.Fprintf(&code, "PROGRAM %s\n\n", pou.Name)
		for _, module := range pou.Modules {
			fmt.Fprintf(&code, "(* %s *)\n", strings.ReplaceAll(module.Name, "-", "_"))
			prefix := bindingPrefix(pou.Kind, ctx.PhysicalProfile, *module.ID)
			ps.IOModules = append(ps.IOModules, xmlartifact.IOModuleSummary{Type: pou.Kind, ID: *module.ID, BindingPrefix: prefix, InstanceName: module.Name, Capacity: module.Capacity, SignalCount: len(module.Channels)})
			if pou.Kind == "DI" {
				hasDI = true
				tag := moduleid.ModuleInstanceTag(plan.ControllerName, module.Name)
				fmt.Fprintf(&code, "%s.Stat := ANY_TO_DWORD (QUAL_STAT(%s_0_VAL.Quality));\n", tag, prefix)
				for channel := 0; channel < module.Capacity; channel++ {
					fmt.Fprintf(&code, "%s.i%02d := %s_%d_VAL.Measurement;\n", tag, channel, prefix, channel)
				}
			} else if pou.Kind == "DO" {
				for channel := 0; channel < module.Capacity; channel++ {
					code.WriteString(doAssignment(plan.ControllerName, module, channel, ctx.PhysicalProfile) + "\n")
				}
			}
			for _, channel := range module.Channels {
				if pou.Kind == "AI" {
					hasAI = true
					if ctx.PhysicalProfile == addressing.PhysicalProfileLegacy {
						fmt.Fprintf(&code, "%s.Xin := %s_%d.ValueDINT;\n%s.Xs := %s_%d.Status;\n", channel.Tag, prefix, channel.Channel, channel.Tag, prefix, channel.Channel)
					} else {
						fmt.Fprintf(&code, "%s.Xin := %s_%d_VAL.Measurement;\n%s.Xs := QUAL_STAT(%s_%d_VAL.Quality);\n", channel.Tag, prefix, channel.Channel, channel.Tag, prefix, channel.Channel)
					}
				}
				moduleID, channelNumber := *module.ID, channel.Channel
				ps.Signals = append(ps.Signals, xmlartifact.SignalSummary{TemplateKey: "assignments:ST:" + pou.Kind, BaseName: channel.Tag, IOType: pou.Kind, ModuleID: &moduleID, Channel: &channelNumber})
			}
			code.WriteByte('\n')
		}
		code.WriteString("END_PROGRAM")
		doc.POUS.Items = append(doc.POUS.Items, xmlmodel.OutputSTPOU{ID: strconv.FormatInt(id, 10), Name: pou.Name, IsFBD: "0", GroupID: ctx.GroupID, Enabled: "1", Number: number, Code: code.String()})
		summary.POUs = append(summary.POUs, ps)
	}
	data, err := xmlcodec.SerializeSCADAValue(doc)
	if err != nil {
		return xmlartifact.Result{}, err
	}
	var actual xmlmodel.OutputSTDocument
	if err := xml.Unmarshal(bytes.TrimPrefix(data, xmlcodec.Utf8BOM), &actual); err != nil {
		return xmlartifact.Result{}, err
	}
	if !reflect.DeepEqual(actual, doc) || bytes.Contains(data, []byte("<ISAGraf")) || bytes.Contains(data, []byte("<ISACARDSINFO")) {
		return xmlartifact.Result{}, fmt.Errorf("Модули: ST XML изменился при сериализации")
	}
	assignments := 0
	for _, pou := range actual.POUS.Items {
		assignments += strings.Count(pou.Code, ":=")
	}
	if assignments != plan.AssignmentCount {
		return xmlartifact.Result{}, fmt.Errorf("Модули: неверное число ST присваиваний")
	}
	warnings := append([]string(nil), plan.Warnings...)
	warnings = append(warnings, "ST назначений сохраняет перечисленные в Excel каналы AI и назначает все каналы добавленных резервных AI-модулей. DO назначает все 32 физических выхода каждого модуля через D32V_v1._00…_31 независимо от заполнения карты FBD. DI назначает Stat и все 32 входа i00…i31 каждого D32V_v1, включая каналы без FBD-получателя. Объекты AD3_v2 и D32V_v1 должны существовать.",
		fmt.Sprintf("Выбран %s, физический профиль %s. Сверьте адреса и числовые ID модулей с конфигурацией целевого проекта.", ctx.ControllerTypeName, ctx.PhysicalProfile))
	if ctx.PhysicalProfile == addressing.PhysicalProfileLegacy {
		warnings = append(warnings, "ST профиля legacy-iu-qu использует ValueDINT/Status для AI, подтверждённые AI715_st.xml; DO использует Value по нативным FBD-связям. Отдельный нативный DO ST-эталон не предоставлен. Импорт, компиляция и выполнение требуют проверки в целевой SCADA.")
	} else if hasAI || hasDI {
		warnings = append(warnings, "Функция QUAL_STAT должна присутствовать в TenixRtLib.")
	}
	if hasDI {
		warnings = append(warnings, "DI ST использует ANY_TO_DWORD для преобразования QUAL_STAT канала 0 в Stat модуля; проверьте наличие обеих функций и выполнение ST перед соответствующим FBD.")
	}
	return xmlartifact.Result{XML: data, BaseName: "MODULE_ASSIGNMENTS_ST", Summary: summary, Warnings: warnings}, nil
}
