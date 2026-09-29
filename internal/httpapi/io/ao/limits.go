// Пределы входного плана до выделения идентификаторов.
package ioao

import (
	"fmt"
	"scheme-xml-generator/internal/aomap"

	"scheme-xml-generator/internal/httpapi/limits"
)

const MaxAOMappingFileBytes int64 = 2 << 20

// CheckAOMappingSize проверяет пределы AO-плана для вызывающих FBD, ST и HMI обработчиков.
// Сверяет число групп, модулей и каналов с общими лимитами HTTP-запроса.
func CheckAOMappingSize(plan *aomap.Plan) error {
	if len(plan.Groups) > limits.MaxDocumentPOUs || plan.ModuleCount > limits.MaxDocumentModules || plan.ChannelCount > limits.MaxDocumentSignals {
		return fmt.Errorf("карта AO превышает пределы одного запроса: %d POU, %d модулей, %d каналов", limits.MaxDocumentPOUs, limits.MaxDocumentModules, limits.MaxDocumentSignals)
	}
	return nil
}
