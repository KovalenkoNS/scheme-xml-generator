// Разбор обозначения модуля A<крейt>-<слот> конкретного входного формата; не вычисляет ModuleID.
package fields

import (
	"fmt"
	"regexp"
	"strconv"
)

var modulePattern = regexp.MustCompile(`^A([0-9]{1,6})[-_]([0-9]{1,4})$`)

// NormalizeModule возвращает каноническое имя, префикс крейта и номер слота входной строки.
func NormalizeModule(value string) (string, string, int, error) {
	parts := modulePattern.FindStringSubmatch(value)
	if parts == nil {
		return "", "", 0, fmt.Errorf("неверное имя модуля %q: ожидается A1-00", value)
	}
	rack, _ := strconv.Atoi(parts[1])
	slot, _ := strconv.Atoi(parts[2])
	if rack < 1 || slot > 4095 {
		return "", "", 0, fmt.Errorf("неверный крейт или номер модуля %q: допустимы номера 0…4095", value)
	}
	prefix := fmt.Sprintf("A%d", rack)
	return fmt.Sprintf("%s-%02d", prefix, slot), prefix, slot, nil
}
