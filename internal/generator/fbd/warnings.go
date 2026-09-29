// Single-template FBD warnings stage: Records portable-context and normalized-name notices for this generation.
package fbd

import (
	"fmt"

	"strings"
)

// prepareWarnings Records portable-context and normalized-name notices for this generation.
// Warnings accompany the saved XML and never alter its graph or destination.
func (b *singleBuild) prepareWarnings() error {
	b.warnings = []string{
		"T11ID, cardId и POU ID являются транспортными; при импорте в существующий проект проверьте отсутствие конфликтов.",
	}
	if strings.TrimSpace(b.generator.Config.Common.Project) == "" || strings.TrimSpace(b.generator.Config.Common.ControllerType) == "" || b.controllerID == "0" || b.resourceID == "0" || b.groupID == "0" || b.pouNumber == "0" {
		b.warnings = append(b.warnings, "Контекст импорта содержит portable-значения (пустой Project/Controller или нулевой GroupID/POUNum). Для целевого проекта заполните config.json либо расширенные поля POU.")
	}
	if b.preview.MatchedPrefix != "" {
		b.warnings = append(b.warnings, fmt.Sprintf("Из введённого имени снят суффикс %s; базовое имя: %s.", b.preview.MatchedPrefix, b.preview.BaseName))
	}

	return nil
}

// appendOnce Добавляет предупреждение в сводку FBD только при отсутствии такой строки.
// Сохраняет порядок сообщений и не меняет уже добавленные предупреждения.
func appendOnce(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

// uniqueStrings Убирает повторные предупреждения объединённого FBD-документа.
// Возвращает строки в порядке первого появления.
func uniqueStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{})
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
