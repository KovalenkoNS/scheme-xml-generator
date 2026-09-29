// Module identity rules resolve names and slots without inferring physical hardware IDs.
package modules

import (
	"fmt"

	"strconv"
	"strings"
)

// AoSTModuleIndex Разбирает физический номер AO-модуля из имени и префикса.
// Возвращает индекс для раскладки ST либо ошибку неподходящего имени.
func AoSTModuleIndex(name, prefix string) (int, error) {
	if !strings.HasPrefix(name, prefix+"_") {
		return 0, fmt.Errorf("модуль не относится к группе")
	}
	suffix := strings.TrimPrefix(name, prefix+"_")
	if suffix == "" || strings.Trim(suffix, "0123456789") != "" {
		return 0, fmt.Errorf("неверный номер модуля")
	}
	index, err := strconv.ParseInt(suffix, 10, 32)
	return int(index), err
}

// ModuleSlot Разбирает номер слота из имени модуля и известного префикса карты.
// Используется подготовкой планов, не подменяет фактический ModuleID.
func ModuleSlot(name, prefix string) (int, bool) {
	parts := RackSlotPattern.FindStringSubmatch(name)
	if len(parts) != 3 || parts[1] != prefix {
		return 0, false
	}
	slot, err := strconv.Atoi(parts[2])
	return slot, err == nil && slot <= 4095 && name == fmt.Sprintf("%s-%02d", prefix, slot)
}
