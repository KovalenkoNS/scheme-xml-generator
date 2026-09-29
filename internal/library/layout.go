// Границы фактической геометрии библиотечного шаблона до смещения экземпляра.
package library

import "strings"

// LayoutBounds describes actual template content before an instance offset is
// applied. The declared WIDTH/HEIGHT intentionally stay separate: they size a
// page, while content bounds control collision-free instance stacking.
type LayoutBounds struct {
	MinX int `json:"minX"`
	MinY int `json:"minY"`
	MaxX int `json:"maxX"`
	MaxY int `json:"maxY"`
}

// TemplateLayoutBounds вычисляет реальные границы содержимого библиотечного шаблона для размещения.
// Учитывает блоки, графику и PointList; при пустом содержимом возвращает заявленные размеры шаблона.
func TemplateLayoutBounds(template *Template) LayoutBounds {
	if template == nil {
		return LayoutBounds{}
	}
	bounds := LayoutBounds{}
	hasContent := false
	// Добавляет координату в общие границы содержимого; вызывается для блоков и PointList.
	// Изменяет bounds и отмечает наличие фактической геометрии шаблона.
	includePoint := func(x, y int) {
		hasContent = true
		bounds.MinX = min(bounds.MinX, x)
		bounds.MinY = min(bounds.MinY, y)
		bounds.MaxX = max(bounds.MaxX, x)
		bounds.MaxY = max(bounds.MaxY, y)
	}
	// Включает оба угла графического объекта в итоговые границы шаблона.
	// Использует ширину/высоту из XML и общий аккумулятор includePoint.
	includeRectangle := func(x, y, width, height int) {
		includePoint(x, y)
		includePoint(x+width, y+height)
	}

	for _, primitive := range template.Contents.Primitives {
		objectType := strings.TrimSpace(primitive.ObjectType)
		params := ParseParams(primitive.Params)
		points, pointsErr := ParsePoints(params["PL"])
		if objectType == "20" {
			if pointsErr == nil {
				for _, point := range points {
					includePoint(point.X, point.Y)
				}
			}
			continue
		}
		if objectType == "7" && pointsErr == nil {
			for _, point := range points {
				includePoint(point.X, point.Y)
			}
			continue
		}
		includeRectangle(Int(primitive.X, 0), Int(primitive.Y, 0), Int(primitive.Width, 0), Int(primitive.Height, 0))
		if pointsErr == nil {
			for _, point := range points {
				includePoint(point.X, point.Y)
			}
		}
	}
	if !hasContent {
		bounds.MaxX = max(0, Int(template.Width, 0))
		bounds.MaxY = max(0, Int(template.Height, 0))
	}
	return bounds
}
