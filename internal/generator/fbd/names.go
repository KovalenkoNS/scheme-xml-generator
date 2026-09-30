// Single-template FBD names stage: library-backed output preparation and assembly.
package fbd

import (
	"fmt"
	fbdrequest "scheme-xml-generator/internal/generator/fbd/request"
	"scheme-xml-generator/internal/generator/identifiers"
	"scheme-xml-generator/internal/library"
	"sort"
	"strings"
	"unicode"
)

// PreviewName Показывает имена будущего экземпляра и его карточек по выбранному шаблону.
// Использует режим имени и введённый тег, возвращает ошибки без выделения ID.
func PreviewName(ref *library.TemplateRef, objectName, mode string) (fbdrequest.NamePreview, error) {
	baseName, matched, err := normalizeObjectName(ref, objectName, mode)
	if err != nil {
		return fbdrequest.NamePreview{}, err
	}
	cardIndex := sourceCards(ref)
	seen := make(map[string]struct{})
	names := make([]string, 0)
	for _, primitive := range ref.Template.Contents.Primitives {
		cardID := strings.TrimSpace(primitive.CardID)
		if cardID == "" || cardID == "0" {
			continue
		}
		if _, ok := seen[cardID]; ok {
			continue
		}
		seen[cardID] = struct{}{}
		card, ok := cardIndex[cardID]
		if !ok {
			return fbdrequest.NamePreview{}, fmt.Errorf("CARDID %s отсутствует в ISAOBJLIST объекта %s", cardID, ref.Owner.Name)
		}
		names = append(names, baseName+card.EffectivePrefix())
	}
	return fbdrequest.NamePreview{BaseName: baseName, MatchedPrefix: matched, ObjectNames: names}, nil
}

// normalizeObjectName Проверяет введённое имя экземпляра с учётом режима и имени библиотечного типа.
// Возвращает базовое имя и итоговую привязку карточки без изменения шаблона.
func normalizeObjectName(ref *library.TemplateRef, value, mode string) (string, string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return "", "", fmt.Errorf("имя объекта не задано")
	}
	if err := identifiers.ValidateIdentifier(value); err != nil {
		return "", "", err
	}
	if mode == "base" {
		return value, "", nil
	}
	index := sourceCards(ref)
	prefixes := make([]string, 0)
	seen := make(map[string]struct{})
	for _, primitive := range ref.Template.Contents.Primitives {
		card, ok := index[strings.TrimSpace(primitive.CardID)]
		if !ok {
			continue
		}
		prefix := card.EffectivePrefix()
		if prefix == "" {
			continue
		}
		if _, ok := seen[prefix]; !ok {
			seen[prefix] = struct{}{}
			prefixes = append(prefixes, prefix)
		}
	}
	sort.Slice(prefixes, func(i, j int) bool { return len(prefixes[i]) > len(prefixes[j]) })
	for _, prefix := range prefixes {
		if strings.HasSuffix(value, prefix) && len(value) > len(prefix) {
			base := strings.TrimSuffix(value, prefix)
			if err := identifiers.ValidateIdentifier(base); err != nil {
				return "", "", err
			}
			return base, prefix, nil
		}
	}
	return value, "", nil
}

// resolvePOUName Выбирает и проверяет имя одиночной FBD-POU из запроса либо шаблона.
// Автоматическое имя формируется только при отсутствии пользовательского.
func resolvePOUName(requested, templateName string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return makePOUName(templateName), nil
	}
	if err := identifiers.ValidatePOUName(requested); err != nil {
		return "", err
	}
	return requested, nil
}

// makePOUName Преобразует название библиотечного шаблона в допустимое имя POU.
// Используется при автоматическом именовании одиночного FBD-документа.
func makePOUName(templateName string) string {
	value := "POU_" + templateName
	var builder strings.Builder
	lastUnderscore := false
	for _, r := range value {
		valid := unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
		if !valid {
			r = '_'
		}
		if r == '_' && lastUnderscore {
			continue
		}
		builder.WriteRune(r)
		lastUnderscore = r == '_'
	}
	result := strings.TrimRight(builder.String(), "_")
	if result == "" || result == "POU" {
		return "Generated_POU"
	}
	if len([]rune(result)) > 160 {
		result = string([]rune(result)[:160])
		result = strings.TrimRight(result, "_")
	}
	return result
}
