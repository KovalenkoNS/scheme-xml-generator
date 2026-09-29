// Диагностический технологический объект и отекстовки физического модуля.
package techobjects

import (
	"fmt"
	"scheme-xml-generator/internal/iomap"

	"strings"
)

// diagnosticObject создаёт технологический объект диагностики указанного физического модуля.
// Получает имя ПЛК и IO-модуль, возвращает тег/тип/шаблон и отекстовки каналов/групп для XLS.
func diagnosticObject(controllerName string, module iomap.Module) Object {
	tag := "_" + controllerName + "_" + module.Name
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
	object.Name = "_" + controllerName + "_IO_" + module.Type + "_" + module.Name
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
