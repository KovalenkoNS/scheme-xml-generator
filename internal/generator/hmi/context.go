// Общий контекст HMI описывает назначение панелей оператора независимо от направления IO.
package hmi

import (
	"fmt"
	"scheme-xml-generator/internal/generator/addressing"
	"scheme-xml-generator/internal/generator/contracts"
	"scheme-xml-generator/internal/generator/xmlcodec"
	"strings"
	"unicode/utf8"
)

// HMIContext содержит общие атрибуты BufScada и номер ресурса в привязках CARDSINFO.
// ResourceNumber не является ResuorceID программного XML.
type HMIContext struct {
	Version        string `json:"version"`
	Project        string `json:"project"`
	ResourceNumber string `json:"resourceNumber"`
}

// DefaultHMIContext возвращает переносимые исходные настройки профиля панелей.
// Имя рабочего проекта остаётся пустым до явного выбора пользователем.
func DefaultHMIContext() HMIContext {
	return HMIContext{Version: "29", ResourceNumber: "1"}
}

// normalizeHMIContext проверяет числа и XML-текст перед построением любых диагностических страниц.
// Возвращает независимую копию контекста либо ошибку, не выбирая AO/AI/DI/DO профиль.
func normalizeHMIContext(ctx HMIContext) (HMIContext, error) {
	for _, field := range []struct {
		name  string
		value *string
	}{{"VER", &ctx.Version}, {"номер ресурса", &ctx.ResourceNumber}} {
		value, err := addressing.NormalizeContextInteger(*field.value, field.name)
		if err != nil {
			return ctx, err
		}
		*field.value = value
	}
	if ctx.ResourceNumber == "0" {
		return ctx, fmt.Errorf("диагностика: номер ресурса в пути привязки должен быть от 1 до %d", contracts.MaxTransportID)
	}
	if !utf8.ValidString(ctx.Project) || len(ctx.Project) > 2048 || strings.ContainsAny(ctx.Project, "\r\n\t") {
		return ctx, fmt.Errorf("диагностика: некорректное значение Project")
	}
	for _, r := range ctx.Project {
		if !xmlcodec.ValidXMLRune(r) {
			return ctx, fmt.Errorf("диагностика: Project содержит недопустимый XML-символ")
		}
	}
	return ctx, nil
}
