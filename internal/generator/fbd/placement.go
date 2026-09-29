// Single-template FBD placement stage: Resolves explicit or automatic offsets from the actual template bounds.
package fbd

import (
	"fmt"

	"scheme-xml-generator/internal/library"
)

// preparePlacement Resolves explicit or automatic offsets from the actual template bounds.
// The original source geometry stays unchanged and excessive offsets are rejected.
func (b *singleBuild) preparePlacement() error {
	b.layout = library.TemplateLayoutBounds(b.ref.Template)
	b.dx, b.dy = 300-min(0, b.layout.MinX), 100-min(0, b.layout.MinY)
	if b.request.OffsetX != nil {
		b.dx = *b.request.OffsetX
	}
	if b.request.OffsetY != nil {
		b.dy = *b.request.OffsetY
	}
	if b.dx < -10000 || b.dx > 100000 || b.dy < -10000 || b.dy > 100000 {
		return fmt.Errorf("offset должен находиться в диапазоне -10000..100000")
	}
	return nil
}

// pointBounds Вычисляет прямоугольные границы точек графического примитива FBD.
// Размеры используются при переносе библиотечной геометрии в выходной XML.
func pointBounds(points []library.Point) (x, y, width, height int) {
	if len(points) == 0 {
		return 0, 0, 0, 0
	}
	minX, maxX := points[0].X, points[0].X
	minY, maxY := points[0].Y, points[0].Y
	for _, point := range points[1:] {
		minX, maxX = min(minX, point.X), max(maxX, point.X)
		minY, maxY = min(minY, point.Y), max(maxY, point.Y)
	}
	return minX, minY, maxX - minX, maxY - minY
}
