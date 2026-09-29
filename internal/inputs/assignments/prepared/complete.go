// Завершение подготовленного формата: одна запись AI собирается из пары выражений Xin/Xs.
package prepared

import (
	"fmt"
	"scheme-xml-generator/internal/inputs/assignments/assembly"
	"strings"
)

// ValidateComplete вызывается после всех листов, поскольку части пары могут приходить из разных строк.
// Проверяет маску именно подготовленного формата до преобразования в независимую модель назначений.
func ValidateComplete(modules map[string]*assembly.Module, controllerNames map[string]string) error {
	for _, parsed := range modules {
		if !parsed.PreparedAIPair {
			continue
		}
		for _, channel := range parsed.Channels {
			if channel.Parts != 3 {
				controller := controllerNames[strings.ToUpper(parsed.Group.ControllerName)]
				return fmt.Errorf("Назначения, строка %d: для %s/%s, канал %d нужны оба назначения Xin и Xs", channel.Channel.SourceRow, controller, parsed.Module.Name, channel.Channel.Channel)
			}
		}
	}
	return nil
}
