// Single-template FBD graphics stage: Clones non-block graphical primitives with their authoritative point geometry.
package fbd

import (
	"fmt"

	"scheme-xml-generator/internal/generator/xmlmodel"
	"scheme-xml-generator/internal/library"

	"strconv"
	"strings"
)

// buildGraphics Clones non-block graphical primitives with their authoritative point geometry.
// Points and bounds move together while source scripts and style values remain unchanged.
func (b *singleBuild) buildGraphics() error {
	b.graphics = make([]xmlmodel.OutputPrimitive, 0, b.graphicCount)
	for _, primitive := range b.primitives {
		if !library.IsGraphicType(primitive.ObjectType) {
			continue
		}
		newID := strconv.FormatInt(b.nextT11, 10)
		b.nextT11++
		x := library.Int(primitive.X, 0) + b.dx
		y := library.Int(primitive.Y, 0) + b.dy
		width := library.Int(primitive.Width, 0)
		height := library.Int(primitive.Height, 0)
		paramsRaw := strings.ReplaceAll(primitive.Params, "\r\n", "\n")
		params := library.ParseParams(paramsRaw)
		if rawPoints, ok := params["PL"]; ok && strings.TrimSpace(rawPoints) != "" {
			shifted, err := library.ShiftPoints(rawPoints, b.dx, b.dy)
			if err != nil {
				return fmt.Errorf("OnePrim ID=%s: %w", primitive.ID, err)
			}
			paramsRaw = library.ReplaceParam(paramsRaw, "PL", shifted)
			if primitive.ObjectType == "7" {
				points, _ := library.ParsePoints(shifted)
				x, y, width, height = pointBounds(points)
				b.warnings = appendOnce(b.warnings, "Для OBJTYPE=7 служебная рамка рассчитана по PointList; сами точки являются авторитетной геометрией.")
			}
		}
		b.graphics = append(b.graphics, xmlmodel.OutputPrimitive{
			SourceT11ID: newID, X: strconv.Itoa(x), Y: strconv.Itoa(y), Width: strconv.Itoa(width), Height: strconv.Itoa(height),
			ObjectType: primitive.ObjectType, GraphicNo: cleanNumber(primitive.GraphicNumber, "0"), DrawType: cleanNumber(primitive.DrawType, "0"),
			PenParams: cleanNumber(primitive.PenParams, "0"), PenColor: cleanNumber(primitive.PenColor, "0"), BrushColor: cleanNumber(primitive.BrushColor, "16777215"),
			Gradient: cleanNumber(primitive.GradientColor, "536870911"), ScriptName: primitive.Name, Params: paramsRaw,
		})
	}

	return nil
}
