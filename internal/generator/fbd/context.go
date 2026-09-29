// Контекст FBD проверяет совместимость физических привязок и применяет настройки только к текущему запросу.
package fbd

import (
	"fmt"
	"scheme-xml-generator/internal/generator/addressing"
	"scheme-xml-generator/internal/generator/contracts"
	cpuprofile "scheme-xml-generator/internal/generator/controller"
	"strings"
	"unicode/utf8"
)

// withGenerationContext Создаёт копию FBD-генератора с константами текущего HTTP-запроса.
// Проверяет CPU и поля Common; конфигурационный файл пользователя не меняет.
func (g Generator) withGenerationContext(ctx *contracts.GenerationContext) (Generator, error) {
	if ctx == nil {
		return g, nil
	}
	cpu, err := cpuprofile.NormalizeSupportedControllerType(ctx.ControllerTypeName)
	if err != nil {
		return g, err
	}
	g.Config.Common.ControllerType = cpu
	// Overrides belong to this request only. An absent value preserves config;
	// an explicit empty project is valid for a portable SCADA export.
	for _, field := range []struct {
		name   string
		value  *string
		target *string
	}{
		{"VER", ctx.Version, &g.Config.Common.Version},
		{"ControllerID", ctx.ControllerID, &g.Config.Common.ControllerID},
		{"ResuorceID", ctx.ResourceID, &g.Config.Common.ResourceID},
	} {
		if field.value != nil {
			value, err := addressing.NormalizeContextInteger(*field.value, field.name)
			if err != nil {
				return g, err
			}
			*field.target = value
		}
	}
	if ctx.Project != nil {
		value := *ctx.Project
		if !utf8.ValidString(value) || len(value) > 2048 || strings.ContainsAny(value, "\x00\r\n\t") {
			return g, fmt.Errorf("некорректное имя проекта")
		}
		for _, char := range value {
			if char < 32 || char == 0xfffe || char == 0xffff {
				return g, fmt.Errorf("имя проекта содержит недопустимый XML-символ")
			}
		}
		g.Config.Common.Project = value
	}
	return g, nil
}
