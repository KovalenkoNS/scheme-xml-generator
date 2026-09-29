// Правила безопасного и различимого имени сохраняемого результата.
package output

import (
	"fmt"
	"path/filepath"

	"os"

	"strings"

	"unicode"
)

// SafeOutputName очищает пользовательское имя перед сохранением результата предметного компонента.
// Удаляет пути/недопустимые символы, ограничивает длину и возвращает XML-имя с безопасным fallback.
func SafeOutputName(requested, baseName, templateName string) string {
	name := strings.TrimSpace(requested)
	if name == "" {
		name = baseName + "_" + templateName
	}
	name = strings.TrimSuffix(name, filepath.Ext(name))
	var builder strings.Builder
	lastSeparator := false
	for _, r := range name {
		valid := unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-'
		if !valid {
			r = '_'
		}
		if r == '_' && lastSeparator {
			continue
		}
		builder.WriteRune(r)
		lastSeparator = r == '_'
		if builder.Len() >= 120 {
			break
		}
	}
	name = strings.Trim(builder.String(), "_.-")
	if name == "" {
		name = "generated_scheme"
	}
	return name + ".xml"
}

// uniqueOutputName выбирает незанятое имя в каталоге результатов под блокировкой Store.
// Проверяет исходное имя и числовые суффиксы, не перезаписывая существующие пользовательские файлы.
func uniqueOutputName(directory, name string) (string, error) {
	extension := filepath.Ext(name)
	base := strings.TrimSuffix(name, extension)
	for index := 1; index < 10000; index++ {
		candidate := name
		if index > 1 {
			candidate = fmt.Sprintf("%s-%d%s", base, index, extension)
		}
		_, err := os.Stat(filepath.Join(directory, candidate))
		if os.IsNotExist(err) {
			return candidate, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("не удалось подобрать свободное имя результата")
}
