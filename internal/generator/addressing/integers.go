// Import addressing validates transport values and confirmed physical driver profiles.
package addressing

import (
	"fmt"
	"strconv"
	"strings"
)

// NormalizeContextInteger Нормализует неотрицательную signed32-константу транспортного XML-контекста.
// Пустое конфигурационное значение становится нулём; некорректная строка даёт ошибку.
func NormalizeContextInteger(value, field string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "0", nil
	}
	number, err := strconv.ParseInt(value, 10, 32)
	if err != nil || number < 0 {
		return "", fmt.Errorf("%s должен быть неотрицательным signed 32-bit целым", field)
	}
	return strconv.FormatInt(number, 10), nil
}
