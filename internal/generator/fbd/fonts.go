// Single-template FBD fonts stage: library-backed output preparation and assembly.
package fbd

import (
	"scheme-xml-generator/internal/generator/xmlmodel"
	"scheme-xml-generator/internal/library"
	"sort"

	"strings"
)

// buildFonts Собирает используемые шаблоном шрифты для выходного FBD-документа.
// Возвращает записи и предупреждения о недостающих библиотечных стилях.
func buildFonts(ref *library.TemplateRef) ([]xmlmodel.OutputFontStyle, []string) {
	styles := make(map[string]xmlmodel.OutputFontStyle)
	warnings := []string{}
	for _, primitive := range ref.Template.Contents.Primitives {
		if !library.IsGraphicType(primitive.ObjectType) {
			continue
		}
		params := library.ParseParams(primitive.Params)
		fontID := strings.TrimSpace(params["FONTID"])
		if fontID == "" || fontID == "65535" {
			continue
		}
		if _, exists := styles[fontID]; exists {
			continue
		}
		if source, ok := ref.Library.FontStyles[fontID]; ok {
			styles[fontID] = xmlmodel.OutputFontStyle{ID: source.ID, Name: source.Name, FontName: source.FontName, FontColor: source.FontColor, FontSize: source.FontSize, FontParam: source.FontParam}
			continue
		}
		parts := strings.Split(params["USERFONT"], ";")
		if len(parts) >= 4 {
			styleName := "Автоматически восстановленный стиль " + fontID
			if fontID == "81" {
				styleName = "Заголовок 1"
			} else if fontID == "83" {
				styleName = "Комментарий в коде"
			}
			styles[fontID] = xmlmodel.OutputFontStyle{ID: fontID, Name: styleName, FontName: parts[1], FontColor: cleanNumber(parts[3], "0"), FontSize: cleanNumber(parts[0], "10"), FontParam: cleanNumber(parts[2], "0")}
			warnings = append(warnings, "FONTID "+fontID+" отсутствует в FONTSTYLES библиотеки и восстановлен из USERFONT.")
		} else {
			warnings = append(warnings, "FONTID "+fontID+" отсутствует в FONTSTYLES и не содержит пригодный USERFONT.")
		}
	}
	ids := make([]string, 0, len(styles))
	for id := range styles {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return library.Int(ids[i], 0) < library.Int(ids[j], 0) })
	result := make([]xmlmodel.OutputFontStyle, 0, len(ids))
	for _, id := range ids {
		result = append(result, styles[id])
	}
	return result, warnings
}
