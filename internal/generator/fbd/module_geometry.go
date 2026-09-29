// Геометрия модульного библиотечного FBD: точки портов, связи и границы страницы.
package fbd

import (
	"fmt"
	"scheme-xml-generator/internal/generator/xmlmodel"
	"strconv"
	"strings"
)

type point struct{ X, Y int }

// makeIOLink Создаёт связь между заданными портами двух FBD-блоков.
// Записывает FP/LP и путь из координат, не меняя исходные блоки.
func makeIOLink(first xmlmodel.OutputBlock, firstPin string, last xmlmodel.OutputBlock, lastPin string, start, end point) xmlmodel.OutputLink {
	return xmlmodel.OutputLink{
		Negative: "false", AsPointer: "false", UserEdit: "false", Color: "0", GetBitNum: "-1", ConvertTo: "0",
		PointList:  xmlmodel.OutputPointList{Points: fmt.Sprintf("(%d,%d);(%d,%d);", start.X, start.Y, end.X, end.Y)},
		FirstPoint: xmlmodel.OutputEndpoint{Value: strings.Join([]string{first.T11ID, "False", firstPin, "0,0,100,20"}, "|")},
		LastPoint:  xmlmodel.OutputEndpoint{Last: strings.Join([]string{last.T11ID, "True", lastPin, "0,0,100,20"}, "|")},
	}
}

// blockXY Читает координаты выходного FBD-блока из строковых XML-параметров.
// Используется расчётом физических связей и размеров страницы.
func blockXY(block xmlmodel.OutputBlock) (int, int) {
	return intValue(block.Graphics.X), intValue(block.Graphics.Y)
}

// blockLeftCenter Вычисляет левую центральную точку блока для входящей FBD-связи.
// Опирается на сохранённые координаты и высоту блока.
func blockLeftCenter(block xmlmodel.OutputBlock) point {
	x, y := blockXY(block)
	return point{X: x, Y: y + intValue(block.Graphics.Height)/2}
}

// blockRightCenter Вычисляет правую центральную точку блока для исходящей FBD-связи.
// Опирается на сохранённые размеры и положение блока.
func blockRightCenter(block xmlmodel.OutputBlock) point {
	x, y := blockXY(block)
	return point{X: x + intValue(block.Graphics.Width), Y: y + intValue(block.Graphics.Height)/2}
}

// intValue Преобразует внутренний числовой XML-параметр в координату FBD.
// Вызывается геометрическими расчётами уже сформированных блоков.
func intValue(value string) int {
	parsed, _ := strconv.Atoi(strings.TrimSpace(value))
	return parsed
}

// d32InputY Определяет высоту канального входа D32 в аппаратном графическом профиле.
// Номер канала задаёт точку присоединения входящей FBD-связи.
func d32InputY(y, channel int) int { return y + 60 + channel*20 }

// d32OutputY Определяет высоту канального выхода D32 в аппаратном графическом профиле.
// Номер канала задаёт точку присоединения исходящей FBD-связи.
func d32OutputY(y, channel int) int { return y + 40 + channel*20 }

// maxPOUBlockRight Находит правый край всех текущих блоков POU перед добавлением IO.
// Позволяет разместить физическую полосу без перекрытия библиотечных экземпляров.
func maxPOUBlockRight(pou xmlmodel.OutputPOU) int {
	result := 0
	for _, block := range pou.ISAGraf.Blocks.Items {
		result = max(result, intValue(block.Graphics.X)+intValue(block.Graphics.Width))
	}
	for _, primitive := range pou.Graphics.Items {
		result = max(result, intValue(primitive.X)+intValue(primitive.Width))
	}
	return result
}

// extendPOUForIO Расширяет размеры страницы FBD до границ добавленного IO-слоя.
// Меняет только геометрию страницы сформированной POU.
func extendPOUForIO(pou *xmlmodel.OutputPOU) {
	maxX, maxY := 0, 0
	for _, block := range pou.ISAGraf.Blocks.Items {
		maxX = max(maxX, intValue(block.Graphics.X)+intValue(block.Graphics.Width))
		maxY = max(maxY, intValue(block.Graphics.Y)+intValue(block.Graphics.Height))
	}
	pou.Params.Width = maxNumericString(pou.Params.Width, strconv.Itoa(maxX+100))
	pou.Params.Height = maxNumericString(pou.Params.Height, strconv.Itoa(maxY+100))
}
