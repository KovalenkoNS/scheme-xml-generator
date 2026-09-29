// Проверки габаритов библиотечных шаблонов по реальной геометрии примитивов.
package library

import "testing"

// Проверяет габариты библиотечного шаблона по прямоугольникам и спискам точек, включая отрицательные координаты и
// линии.
func TestTemplateLayoutBoundsIncludesDeclaredRectanglesAndPointLists(t *testing.T) {
	template := &Template{
		Width: "100", Height: "80",
		Contents: Contents{Primitives: []Primitive{
			{ObjectType: "34", X: "90", Y: "-20", Width: "30", Height: "60"},
			{ObjectType: "20", Params: "[PL]=(-15,95);(130,120);"},
			{ObjectType: "7", X: "999", Y: "999", Width: "500", Height: "500", Params: "[PL]=(-25,-30);(40,140);"},
		}},
	}
	want := (LayoutBounds{MinX: -25, MinY: -30, MaxX: 130, MaxY: 140})
	if got := TemplateLayoutBounds(template); got != want {
		t.Fatalf("bounds=%+v want %+v", got, want)
	}
}

// Проверяет пустой шаблон библиотеки: при отсутствии примитивов границы берутся из явно объявленного полотна.
func TestTemplateLayoutBoundsFallsBackToDeclaredCanvasWhenEmpty(t *testing.T) {
	template := &Template{Width: "640", Height: "480"}
	want := (LayoutBounds{MinX: 0, MinY: 0, MaxX: 640, MaxY: 480})
	if got := TemplateLayoutBounds(template); got != want {
		t.Fatalf("bounds=%+v want %+v", got, want)
	}
}
