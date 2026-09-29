// Single-template FBD links stage: Rewrites template endpoints to generated block IDs and shifts each route.
package fbd

import (
	"fmt"

	"scheme-xml-generator/internal/generator/xmlmodel"
	"scheme-xml-generator/internal/library"

	"strconv"
	"strings"
)

// buildLinks Rewrites template endpoints to generated block IDs and shifts each route.
// Every source link consumes its transport slot and preserves its pin direction.
func (b *singleBuild) buildLinks() error {
	b.links = make([]xmlmodel.OutputLink, 0, b.linkCount)
	for _, primitive := range b.primitives {
		if !library.IsLinkType(primitive.ObjectType) {
			continue
		}
		b.nextT11++ // Связь потребляет внутренний T11ID, хотя он не сериализуется.
		params := library.ParseParams(primitive.Params)
		points, err := library.ShiftPoints(params["PL"], b.dx, b.dy)
		if err != nil {
			return fmt.Errorf("grprim link ID=%s: %w", primitive.ID, err)
		}
		first, err := rewriteEndpoint(params["FP"], b.idMap)
		if err != nil {
			return fmt.Errorf("FP связи ID=%s: %w", primitive.ID, err)
		}
		last, err := rewriteEndpoint(params["LP"], b.idMap)
		if err != nil {
			return fmt.Errorf("LP связи ID=%s: %w", primitive.ID, err)
		}
		b.links = append(b.links, xmlmodel.OutputLink{
			Negative: "false", AsPointer: "false", UserEdit: strconv.FormatBool(strings.TrimSpace(primitive.DrawType) == "1"),
			Color: cleanNumber(primitive.PenColor, "0"), GetBitNum: cleanNumber(params["BN"], "-1"), ConvertTo: cleanNumber(params["CT"], "0"),
			PointList: xmlmodel.OutputPointList{Points: points}, FirstPoint: xmlmodel.OutputEndpoint{Value: first}, LastPoint: xmlmodel.OutputEndpoint{Last: last},
		})
	}

	return nil
}

// rewriteEndpoint Заменяет исходный ID блока в конце связи FBD по карте новых ID.
// Сохраняет порт и координатную часть; неизвестная ссылка даёт ошибку.
func rewriteEndpoint(raw string, idMap map[string]string) (string, error) {
	parts := strings.SplitN(strings.TrimSpace(raw), "|", 4)
	if len(parts) != 4 {
		return "", fmt.Errorf("неверный endpoint %q", raw)
	}
	newID, ok := idMap[strings.TrimSpace(parts[0])]
	if !ok {
		return "", fmt.Errorf("endpoint ссылается на неизвестный block ID=%s", parts[0])
	}
	direction := ""
	switch strings.ToLower(strings.TrimSpace(parts[1])) {
	case "0", "false":
		direction = "False"
	case "-1", "true":
		direction = "True"
	default:
		return "", fmt.Errorf("неизвестное направление endpoint %q", parts[1])
	}
	return strings.Join([]string{newID, direction, parts[2], "0,0,100,20"}, "|"), nil
}
