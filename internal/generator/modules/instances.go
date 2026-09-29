// Module identity rules resolve names and slots without inferring physical hardware IDs.
package modules

import (
	"strings"
)

// ModuleInstanceTag Формирует имя экземпляра модуля из имени ПЛК и физического модуля.
// Общий формат связывает ST-перекладки и FBD-блоки.
func ModuleInstanceTag(scs, name string) string {
	return "_" + scs + "_" + strings.ReplaceAll(name, "-", "_")
}
