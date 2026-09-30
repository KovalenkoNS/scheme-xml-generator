// Инверсия клонирует библиотечный граф и вставляет NOT исключительно на подтверждённую связь BOOL → D32.iNN.
package fbd

import (
	"fmt"
	fbdrequest "scheme-xml-generator/internal/generator/fbd/request"
	"scheme-xml-generator/internal/library"
	"strconv"
	"strings"
)

// invertD32Template подготавливает отдельный библиотечный сигнал с invert=true.
// Копирует TemplateRef и вставляет NOT только на подтверждённые BOOL→D32.iNN связи;
// используется подсчётом ID и генерацией, не изменяет исходную библиотеку и карточки.
func invertD32Template(ref *library.TemplateRef) (*library.TemplateRef, error) {
	primitives := ref.Template.Contents.Primitives
	byID := make(map[string]library.Primitive, len(primitives))
	usedIDs := make(map[string]bool, len(primitives))
	for _, primitive := range primitives {
		id := strings.TrimSpace(primitive.ID)
		if id == "" || usedIDs[id] {
			return nil, fmt.Errorf("инверсия: отсутствует или повторяется ID примитива %q", id)
		}
		usedIDs[id] = true
		byID[id] = primitive
	}
	cards := sourceCards(ref)
	targets := make(map[string]bool)
	type connection struct {
		index         int
		first, last   string
		start, finish library.Point
	}
	connections := make([]connection, 0)
	for index, primitive := range primitives {
		if !library.IsLinkType(primitive.ObjectType) {
			continue
		}
		params := library.ParseParams(primitive.Params)
		first, last := strings.SplitN(strings.TrimSpace(params["FP"]), "|", 4), strings.SplitN(strings.TrimSpace(params["LP"]), "|", 4)
		if len(first) != 4 || len(last) != 4 {
			return nil, fmt.Errorf("инверсия: некорректные концы связи %s", primitive.ID)
		}
		target, ok := byID[last[0]]
		if !ok || target.ObjectType != "37" || strings.TrimSpace(target.TypeName) != "D32V_v1" {
			continue
		}
		// Channel names are evidenced by the library DIO-1 template and the
		// native DO XML; the diagnostic sts port is deliberately excluded.
		if len(last[2]) != 3 || last[2][0] != 'i' || last[2][1] < '0' || last[2][1] > '9' || last[2][2] < '0' || last[2][2] > '9' {
			continue
		}
		channel, _ := strconv.Atoi(last[2][1:])
		if channel > 31 {
			return nil, fmt.Errorf("инверсия: неподтверждённый канал D32 %s", last[2])
		}
		targetCard, ok := cards[strings.TrimSpace(target.CardID)]
		if !ok || strings.TrimSpace(targetCard.TypeName) != "D32V_v1" || strings.TrimSpace(target.ISAObjectID) == "" {
			return nil, fmt.Errorf("инверсия: тип карточки D32 не подтверждён выбранной библиотекой")
		}
		targetKey := last[0] + "|" + last[2]
		if targets[targetKey] {
			return nil, fmt.Errorf("инверсия: вход D32.%s имеет несколько источников", last[2])
		}
		targets[targetKey] = true
		source, ok := byID[first[0]]
		sourceCard, hasCard := cards[strings.TrimSpace(source.CardID)]
		sourceParams := library.ParseParams(source.Params)
		if !ok || source.ObjectType != "31" || !hasCard || strings.TrimSpace(sourceCard.TypeName) != "BOOL" || strings.TrimSpace(sourceParams["TEXT"]) != "" || first[2] != "0" || !strings.EqualFold(first[1], "false") || !strings.EqualFold(last[1], "true") {
			return nil, fmt.Errorf("инверсия: перед D32.%s требуется прямая связь библиотечного BOOL-сигнала с выходом 0", last[2])
		}
		if (strings.TrimSpace(params["BN"]) != "" && strings.TrimSpace(params["BN"]) != "-1") || (strings.TrimSpace(params["CT"]) != "" && strings.TrimSpace(params["CT"]) != "0") {
			return nil, fmt.Errorf("инверсия: связь перед D32.%s содержит выбор бита или преобразование типа", last[2])
		}
		points, err := library.ParsePoints(params["PL"])
		if err != nil || len(points) < 2 {
			return nil, fmt.Errorf("инверсия: связь перед D32.%s не содержит корректной геометрии", last[2])
		}
		connections = append(connections, connection{index: index, first: params["FP"], last: params["LP"], start: points[0], finish: points[len(points)-1]})
	}
	if len(connections) == 0 {
		return nil, fmt.Errorf("инверсия недоступна: выбранный шаблон не содержит прямой связи BOOL-сигнал → D32.i00…i31")
	}

	copyRef, copyTemplate := *ref, *ref.Template
	copyTemplate.Contents.Primitives = append([]library.Primitive(nil), primitives...)
	copyRef.Template = &copyTemplate
	layout := library.TemplateLayoutBounds(ref.Template)
	// New operators occupy fresh rows below the original diagram. This
	// preserves every original primitive's coordinates and avoids overlap.
	newID := func(prefix string, ordinal int) string {
		for {
			id := fmt.Sprintf("__invert_%s_%d", prefix, ordinal)
			if !usedIDs[id] {
				usedIDs[id] = true
				return id
			}
			ordinal++
		}
	}
	for index, link := range connections {
		primitive := primitives[link.index]
		x, y := max(0, link.start.X), layout.MaxY+30+index*50
		if x > fbdrequest.MaxPageExtent-100 || y > fbdrequest.MaxPageExtent-50 {
			return nil, fmt.Errorf("инверсия: схема превышает допустимый размер страницы")
		}
		notID := newID("not", index)
		operator := library.Primitive{
			ID: notID, ObjectType: "35", ISAObjectID: "-32", CardID: "0",
			X: strconv.Itoa(x), Y: strconv.Itoa(y), Width: "80", Height: "20",
			Params: "[TEXT]=\n[CI]=1\n[CO]=1\n[VMODE]=0\n[COMMENT]=False",
		}
		// NOT's kind and ports come from DO_FBD_example.xml; the library
		// determines all other endpoint/type values. There is no card for NOT.
		left := primitive
		left.Params = library.ReplaceParam(left.Params, "LP", notID+"|True|0|0,0,0,0")
		left.Params = library.ReplaceParam(left.Params, "PL", fmt.Sprintf("(%d,%d);(%d,%d);(%d,%d);", link.start.X, link.start.Y, x-20, link.start.Y, x, y+10))
		right := primitive
		right.ID = newID("link", index)
		right.Params = library.ReplaceParam(right.Params, "FP", notID+"|False|Result|0,0,0,0")
		right.Params = library.ReplaceParam(right.Params, "PL", fmt.Sprintf("(%d,%d);(%d,%d);(%d,%d);", x+80, y+10, link.finish.X-20, y+10, link.finish.X, link.finish.Y))
		copyTemplate.Contents.Primitives[link.index] = left
		copyTemplate.Contents.Primitives = append(copyTemplate.Contents.Primitives, operator, right)
	}
	return &copyRef, nil
}
