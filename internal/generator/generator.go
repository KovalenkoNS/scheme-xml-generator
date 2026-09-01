package generator

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"scheme-xml-generator/internal/library"
)

type generatedCard struct {
	Source library.ISAObject
	ID     string
	Info   string
	Name   string
}

func Requirements(ref *library.TemplateRef) (t11Count, cardCount int) {
	cards := make(map[string]struct{})
	for _, primitive := range ref.Template.Contents.Primitives {
		if primitive.CardID != "" && primitive.CardID != "0" {
			cards[primitive.CardID] = struct{}{}
		}
	}
	return len(ref.Template.Contents.Primitives), len(cards)
}

func PreviewName(ref *library.TemplateRef, objectName, mode string) (NamePreview, error) {
	baseName, matched, err := normalizeObjectName(ref, objectName, mode)
	if err != nil {
		return NamePreview{}, err
	}
	cardIndex := sourceCards(ref)
	seen := make(map[string]struct{})
	names := make([]string, 0)
	for _, primitive := range ref.Template.Contents.Primitives {
		cardID := strings.TrimSpace(primitive.CardID)
		if cardID == "" || cardID == "0" {
			continue
		}
		if _, ok := seen[cardID]; ok {
			continue
		}
		seen[cardID] = struct{}{}
		card, ok := cardIndex[cardID]
		if !ok {
			return NamePreview{}, fmt.Errorf("CARDID %s отсутствует в ISAOBJLIST объекта %s", cardID, ref.Owner.Name)
		}
		names = append(names, baseName+card.EffectivePrefix())
	}
	return NamePreview{BaseName: baseName, MatchedPrefix: matched, ObjectNames: names}, nil
}

func (g Generator) Generate(ref *library.TemplateRef, request Request, ids IDRange) (Result, error) {
	preview, err := PreviewName(ref, request.ObjectName, request.NameMode)
	if err != nil {
		return Result{}, err
	}
	pouName, err := resolvePOUName(request.POUName, ref.Template.Name)
	if err != nil {
		return Result{}, err
	}
	if err := validateText(request.Description, "описание"); err != nil {
		return Result{}, err
	}
	if err := validateText(request.ClusterPath, "KLPath"); err != nil {
		return Result{}, err
	}
	dx, dy := 300, 100
	if request.OffsetX != nil {
		dx = *request.OffsetX
	}
	if request.OffsetY != nil {
		dy = *request.OffsetY
	}
	if dx < -10000 || dx > 100000 || dy < -10000 || dy > 100000 {
		return Result{}, fmt.Errorf("offset должен находиться в диапазоне -10000..100000")
	}
	if request.T11Start != nil {
		ids.T11Start = *request.T11Start
	}
	if request.CardStart != nil {
		ids.CardStart = *request.CardStart
	}
	if request.POUID != nil {
		ids.POUID = *request.POUID
	}
	if ids.T11Start < 1 || ids.CardStart < 1 || ids.POUID < 1 {
		return Result{}, fmt.Errorf("начальные ID должны быть положительными")
	}
	const maxTransportID int64 = 2147483647
	if ids.POUID > maxTransportID {
		return Result{}, fmt.Errorf("POU ID должен помещаться в signed 32-bit")
	}
	controllerID, err := normalizeContextInteger(g.Config.Common.ControllerID, "ControllerID")
	if err != nil {
		return Result{}, err
	}
	resourceID, err := normalizeContextInteger(g.Config.Common.ResourceID, "ResuorceID")
	if err != nil {
		return Result{}, err
	}
	groupID, err := normalizeContextInteger(g.Config.Page.GroupID, "POU GroupID")
	if err != nil {
		return Result{}, err
	}
	pouNumber, err := normalizeContextInteger(g.Config.Page.POUNumber, "POUNum")
	if err != nil {
		return Result{}, err
	}
	if request.POUGroupID != nil {
		if *request.POUGroupID < 1 || *request.POUGroupID > maxTransportID {
			return Result{}, fmt.Errorf("POU GroupID должен быть положительным signed 32-bit")
		}
		groupID = strconv.FormatInt(*request.POUGroupID, 10)
	}
	if request.POUNumber != nil {
		if *request.POUNumber < 1 || *request.POUNumber > maxTransportID {
			return Result{}, fmt.Errorf("POUNum должен быть положительным signed 32-bit")
		}
		pouNumber = strconv.FormatInt(*request.POUNumber, 10)
	}

	warnings := []string{
		"T11ID, cardId и POU ID являются транспортными; при импорте в существующий проект проверьте отсутствие конфликтов.",
	}
	if strings.TrimSpace(g.Config.Common.Project) == "" || strings.TrimSpace(g.Config.Common.ControllerType) == "" || controllerID == "0" || resourceID == "0" || groupID == "0" || pouNumber == "0" {
		warnings = append(warnings, "Контекст импорта содержит portable-значения (пустой Project/Controller или нулевой GroupID/POUNum). Для целевого проекта заполните config.json либо расширенные поля POU.")
	}
	if preview.MatchedPrefix != "" {
		warnings = append(warnings, fmt.Sprintf("Из введённого имени снят суффикс %s; базовое имя: %s.", preview.MatchedPrefix, preview.BaseName))
	}

	cards, cardMap, err := buildCards(ref, preview.BaseName, request.Description, ids.CardStart)
	if err != nil {
		return Result{}, err
	}
	for _, card := range cards {
		if card.Source.InitialValue != nil && strings.HasPrefix(*card.Source.InitialValue, "*") {
			warnings = appendOnce(warnings, "INITIALVALUE с ведущим маркером '*' сохранён буквально; семантика маркера должна поддерживаться целевым импортёром.")
		}
	}
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(ref.Template.Description)), "OLD") {
		warnings = append(warnings, "Выбранный шаблон помечен библиотекой как OLD/legacy.")
	}

	primitives := ref.Template.Contents.Primitives
	blockCount, linkCount, graphicCount := 0, 0, 0
	for _, primitive := range primitives {
		switch {
		case library.IsSupportedBlockType(primitive.ObjectType):
			blockCount++
		case library.IsLinkType(primitive.ObjectType):
			linkCount++
		case library.IsGraphicType(primitive.ObjectType):
			graphicCount++
		default:
			return Result{}, fmt.Errorf("шаблон содержит неподдерживаемый GROBJTYPE=%s (grprim ID=%s)", primitive.ObjectType, primitive.ID)
		}
	}
	if blockCount+linkCount+graphicCount != len(primitives) {
		return Result{}, fmt.Errorf("внутренняя ошибка классификации примитивов")
	}
	if ids.T11Start > maxTransportID-int64(len(primitives))+1 {
		return Result{}, fmt.Errorf("диапазон T11ID выходит за signed 32-bit")
	}
	if len(cards) > 0 && ids.CardStart > maxTransportID-int64(len(cards))+1 {
		return Result{}, fmt.Errorf("диапазон cardId выходит за signed 32-bit")
	}

	nextT11 := ids.T11Start
	idMap := make(map[string]string, blockCount)
	orderedBlocks, reorderedBlocks, err := orderBlockPrimitives(primitives, cardMap)
	if err != nil {
		return Result{}, err
	}
	if reorderedBlocks {
		warnings = append(warnings, "Блоки упорядочены по зависимостям карточек: владелец помещён раньше обращений к его полям.")
	}
	blocks := make([]outputBlock, 0, blockCount)
	for _, primitive := range orderedBlocks {
		if _, exists := idMap[primitive.ID]; exists {
			return Result{}, fmt.Errorf("повторный grprim ID=%s", primitive.ID)
		}
		newID := strconv.FormatInt(nextT11, 10)
		nextT11++
		idMap[primitive.ID] = newID
		params := library.ParseParams(primitive.Params)
		cardID := "0"
		var initial *string
		var card *generatedCard
		if sourceID := strings.TrimSpace(primitive.CardID); sourceID != "" && sourceID != "0" {
			resolved, ok := cardMap[sourceID]
			if !ok {
				return Result{}, fmt.Errorf("не разрешён CARDID=%s у grprim ID=%s", sourceID, primitive.ID)
			}
			card = resolved
			cardID = resolved.ID
			if resolved.Source.InitialValue != nil {
				value := *resolved.Source.InitialValue
				initial = &value
			}
		} else if primitive.InitialValue != nil {
			value := *primitive.InitialValue
			initial = &value
		}
		ci, co := cleanNumber(params["CI"], "0"), cleanNumber(params["CO"], "0")
		if primitive.ObjectType == "36" || primitive.ObjectType == "37" {
			if ref.Template.ID == "19963" && primitive.ISAObjectID == "18513" && primitive.TypeName == "AD3_HLB" && library.Int(co, 0) < 17 {
				oldCO := co
				co = "17"
				warnings = append(warnings, fmt.Sprintf("Сигнатура AD3_HLB актуализирована по подтверждённому экспорту шаблона 19963: CO %s→17.", oldCO))
			} else if signature, ok := ref.Library.TypeSignatures[library.SignatureKey(primitive.ISAObjectID, primitive.TypeName)]; ok && (signature.CI != library.Int(ci, 0) || signature.CO != library.Int(co, 0)) {
				warnings = appendOnce(warnings, fmt.Sprintf("Для типа %s в библиотеке найдены разные сигнатуры; сохранены CI=%s, CO=%s выбранного шаблона.", primitive.TypeName, ci, co))
			}
		}
		commented, err := normalizeBool(params["COMMENT"])
		if err != nil {
			return Result{}, fmt.Errorf("grprim block ID=%s: COMMENT: %w", primitive.ID, err)
		}
		blocks = append(blocks, outputBlock{
			ObjectType: primitive.ObjectType,
			Info:       blockInfo(primitive, params["TEXT"], card),
			T11ID:      newID,
			Graphics: outputBlockBounds{
				X: strconv.Itoa(library.Int(primitive.X, 0) + dx), Y: strconv.Itoa(library.Int(primitive.Y, 0) + dy),
				Width: cleanNumber(primitive.Width, "0"), Height: cleanNumber(primitive.Height, "0"),
			},
			Params: outputBlockParams{
				Text: params["TEXT"], CI: ci, CO: co, ViewMode: cleanNumber(params["VMODE"], "0"),
				Commented: commented, ISAObjectID: cleanNumber(primitive.ISAObjectID, "0"), CardID: cardID, Initial: initial,
			},
		})
	}

	links := make([]outputLink, 0, linkCount)
	for _, primitive := range primitives {
		if !library.IsLinkType(primitive.ObjectType) {
			continue
		}
		nextT11++ // Связь потребляет внутренний T11ID, хотя он не сериализуется.
		params := library.ParseParams(primitive.Params)
		points, err := library.ShiftPoints(params["PL"], dx, dy)
		if err != nil {
			return Result{}, fmt.Errorf("grprim link ID=%s: %w", primitive.ID, err)
		}
		first, err := rewriteEndpoint(params["FP"], idMap)
		if err != nil {
			return Result{}, fmt.Errorf("FP связи ID=%s: %w", primitive.ID, err)
		}
		last, err := rewriteEndpoint(params["LP"], idMap)
		if err != nil {
			return Result{}, fmt.Errorf("LP связи ID=%s: %w", primitive.ID, err)
		}
		links = append(links, outputLink{
			Negative: "false", AsPointer: "false", UserEdit: strconv.FormatBool(strings.TrimSpace(primitive.DrawType) == "1"),
			Color: cleanNumber(primitive.PenColor, "0"), GetBitNum: cleanNumber(params["BN"], "-1"), ConvertTo: cleanNumber(params["CT"], "0"),
			PointList: outputPointList{Points: points}, FirstPoint: outputEndpoint{Value: first}, LastPoint: outputEndpoint{Last: last},
		})
	}

	graphics := make([]outputPrimitive, 0, graphicCount)
	for _, primitive := range primitives {
		if !library.IsGraphicType(primitive.ObjectType) {
			continue
		}
		newID := strconv.FormatInt(nextT11, 10)
		nextT11++
		x := library.Int(primitive.X, 0) + dx
		y := library.Int(primitive.Y, 0) + dy
		width := library.Int(primitive.Width, 0)
		height := library.Int(primitive.Height, 0)
		paramsRaw := strings.ReplaceAll(primitive.Params, "\r\n", "\n")
		params := library.ParseParams(paramsRaw)
		if rawPoints, ok := params["PL"]; ok && strings.TrimSpace(rawPoints) != "" {
			shifted, err := library.ShiftPoints(rawPoints, dx, dy)
			if err != nil {
				return Result{}, fmt.Errorf("OnePrim ID=%s: %w", primitive.ID, err)
			}
			paramsRaw = library.ReplaceParam(paramsRaw, "PL", shifted)
			if primitive.ObjectType == "7" {
				points, _ := library.ParsePoints(shifted)
				x, y, width, height = pointBounds(points)
				warnings = appendOnce(warnings, "Для OBJTYPE=7 служебная рамка рассчитана по PointList; сами точки являются авторитетной геометрией.")
			}
		}
		graphics = append(graphics, outputPrimitive{
			SourceT11ID: newID, X: strconv.Itoa(x), Y: strconv.Itoa(y), Width: strconv.Itoa(width), Height: strconv.Itoa(height),
			ObjectType: primitive.ObjectType, GraphicNo: cleanNumber(primitive.GraphicNumber, "0"), DrawType: cleanNumber(primitive.DrawType, "0"),
			PenParams: cleanNumber(primitive.PenParams, "0"), PenColor: cleanNumber(primitive.PenColor, "0"), BrushColor: cleanNumber(primitive.BrushColor, "16777215"),
			Gradient: cleanNumber(primitive.GradientColor, "536870911"), ScriptName: primitive.Name, Params: paramsRaw,
		})
	}

	fonts, fontWarnings := buildFonts(ref)
	warnings = append(warnings, fontWarnings...)
	isaObjects, err := buildISAObjects(ref)
	if err != nil {
		return Result{}, err
	}
	cardRecords := make([]outputISACard, 0, len(cards))
	for _, card := range cards {
		size := cleanNumber(card.Source.Size, "0")
		cardRecords = append(cardRecords, outputISACard{ID: card.ID, Info: card.Info, IsRetain: "1", Name: card.Name, Size: size, ClusterPath: request.ClusterPath})
	}

	pageWidth := max(g.Config.Page.Width, library.Int(ref.Template.Width, 0)+dx+g.Config.Page.MarginRight)
	pageHeight := max(g.Config.Page.Height, library.Int(ref.Template.Height, 0)+dy+g.Config.Page.MarginBottom)
	version := g.Config.Common.Version
	if version == "" {
		version = ref.Library.Version
	}
	var fontStyles *outputFontStyles
	if len(fonts) > 0 {
		fontStyles = &outputFontStyles{Items: fonts}
	}
	document := outputDocument{
		Common: outputCommon{Version: version, Project: g.Config.Common.Project, IsCut: "false", IsFFB: "false", ControllerType: g.Config.Common.ControllerType, ControllerID: controllerID, ResourceID: resourceID},
		POUS: outputPOUS{POU: outputPOU{
			ID: strconv.FormatInt(ids.POUID, 10), Name: pouName, IsFBD: "1", GroupID: groupID, Enabled: "1", Number: pouNumber, Description: "",
			Params:   outputPOUParams{DParams: g.Config.Page.DParams, Height: strconv.Itoa(pageHeight), Width: strconv.Itoa(pageWidth), TemplatePage: "0", Background: g.Config.Page.Background, PrintWidth: "0", PrintHeight: "0", PrintPageA4: "8"},
			ISAGraf:  outputISAGraf{Blocks: outputBlocks{Items: blocks}, Gotos: outputGotos{}, Links: outputLinks{Items: links}},
			Graphics: outputGraphics{Items: graphics},
		}},
		FontStyles: fontStyles, ISAObjects: outputISAObjects{Items: isaObjects}, ISACards: outputISACards{Items: cardRecords},
	}

	data, err := serializeSCADA(document)
	if err != nil {
		return Result{}, fmt.Errorf("сериализовать SCADA XML: %w", err)
	}
	if err := validateGeneratedXML(data, len(primitives), blocks, links, graphics, len(fonts)); err != nil {
		return Result{}, err
	}

	return Result{
		XML: data, BaseName: preview.BaseName, Warnings: uniqueStrings(warnings),
		Summary: Summary{
			Blocks: len(blocks), Links: len(links), Graphics: len(graphics), Cards: len(cards),
			T11First: ids.T11Start, T11Last: nextT11 - 1, CardFirst: ids.CardStart, CardLast: ids.CardStart + int64(len(cards)) - 1, POUID: ids.POUID, POUName: pouName, POUGroupID: groupID, POUNumber: pouNumber,
		},
	}, nil
}

func sourceCards(ref *library.TemplateRef) map[string]library.ISAObject {
	result := make(map[string]library.ISAObject, len(ref.Owner.ISAObjects.Items))
	for _, card := range ref.Owner.ISAObjects.Items {
		result[card.ID] = card
	}
	return result
}

func buildCards(ref *library.TemplateRef, baseName, description string, start int64) ([]*generatedCard, map[string]*generatedCard, error) {
	index := sourceCards(ref)
	ordered := make([]*generatedCard, 0)
	result := make(map[string]*generatedCard)
	for _, primitive := range ref.Template.Contents.Primitives {
		sourceID := strings.TrimSpace(primitive.CardID)
		if sourceID == "" || sourceID == "0" {
			continue
		}
		if _, exists := result[sourceID]; exists {
			continue
		}
		source, ok := index[sourceID]
		if !ok {
			return nil, nil, fmt.Errorf("CARDID %s отсутствует в ISAOBJLIST объекта %s", sourceID, ref.Owner.Name)
		}
		info := baseName + source.EffectivePrefix()
		name := strings.TrimSpace(description)
		if name == "" {
			name = info
		}
		if suffix := strings.TrimSpace(source.Description); suffix != "" {
			name += " " + suffix
		}
		card := &generatedCard{Source: source, ID: strconv.FormatInt(start+int64(len(ordered)), 10), Info: info, Name: name}
		result[sourceID] = card
		ordered = append(ordered, card)
	}
	return ordered, result, nil
}

func blockInfo(primitive library.Primitive, text string, card *generatedCard) string {
	if card != nil {
		if strings.HasPrefix(text, ".") {
			return card.Info + text
		}
		return card.Info
	}
	if strings.TrimSpace(primitive.TypeName) != "" {
		return primitive.TypeName
	}
	if primitive.ObjectType == "35" {
		switch strings.TrimSpace(primitive.ISAObjectID) {
		case "-3":
			return "+"
		case "-4":
			return "/"
		case "-33":
			return "OR"
		}
	}
	return text
}

func rewriteEndpoint(raw string, idMap map[string]string) (string, error) {
	parts := strings.SplitN(strings.TrimSpace(raw), "|", 4)
	if len(parts) != 4 {
		return "", fmt.Errorf("неверный endpoint %q", raw)
	}
	newID, ok := idMap[parts[0]]
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

func buildFonts(ref *library.TemplateRef) ([]outputFontStyle, []string) {
	styles := make(map[string]outputFontStyle)
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
			styles[fontID] = outputFontStyle{ID: source.ID, Name: source.Name, FontName: source.FontName, FontColor: source.FontColor, FontSize: source.FontSize, FontParam: source.FontParam}
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
			styles[fontID] = outputFontStyle{ID: fontID, Name: styleName, FontName: parts[1], FontColor: cleanNumber(parts[3], "0"), FontSize: cleanNumber(parts[0], "10"), FontParam: cleanNumber(parts[2], "0")}
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
	result := make([]outputFontStyle, 0, len(ids))
	for _, id := range ids {
		result = append(result, styles[id])
	}
	return result, warnings
}

func buildISAObjects(ref *library.TemplateRef) ([]outputISAObject, error) {
	objects := make(map[string]outputISAObject)
	for _, primitive := range ref.Template.Contents.Primitives {
		if !library.IsSupportedBlockType(primitive.ObjectType) || library.Int(primitive.ISAObjectID, 0) <= 0 {
			continue
		}
		id := strings.TrimSpace(primitive.ISAObjectID)
		record := outputISAObject{ID: id, Info: primitive.TypeName, LibraryName: primitive.LibraryName}
		if old, exists := objects[id]; exists {
			if old.Info != record.Info || old.LibraryName != record.LibraryName {
				return nil, fmt.Errorf("OBJMSID %s имеет противоречивые определения", id)
			}
			continue
		}
		objects[id] = record
	}
	ids := make([]string, 0, len(objects))
	for id := range objects {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return library.Int(ids[i], 0) < library.Int(ids[j], 0) })
	result := make([]outputISAObject, 0, len(ids))
	for _, id := range ids {
		result = append(result, objects[id])
	}
	return result, nil
}

func normalizeObjectName(ref *library.TemplateRef, value, mode string) (string, string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return "", "", fmt.Errorf("имя объекта не задано")
	}
	if err := validateIdentifier(value); err != nil {
		return "", "", err
	}
	if mode == "base" {
		return value, "", nil
	}
	index := sourceCards(ref)
	prefixes := make([]string, 0)
	seen := make(map[string]struct{})
	for _, primitive := range ref.Template.Contents.Primitives {
		card, ok := index[strings.TrimSpace(primitive.CardID)]
		if !ok {
			continue
		}
		prefix := card.EffectivePrefix()
		if prefix == "" {
			continue
		}
		if _, ok := seen[prefix]; !ok {
			seen[prefix] = struct{}{}
			prefixes = append(prefixes, prefix)
		}
	}
	sort.Slice(prefixes, func(i, j int) bool { return len(prefixes[i]) > len(prefixes[j]) })
	for _, prefix := range prefixes {
		if strings.HasSuffix(value, prefix) && len(value) > len(prefix) {
			base := strings.TrimSuffix(value, prefix)
			if err := validateIdentifier(base); err != nil {
				return "", "", err
			}
			return base, prefix, nil
		}
	}
	return value, "", nil
}

func validateIdentifier(value string) error {
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '.' || r == '-' {
			continue
		}
		return fmt.Errorf("имя объекта содержит недопустимый символ %q", r)
	}
	return nil
}

func validateText(value, field string) error {
	for _, r := range value {
		if r == '\t' || r == '\n' || r == '\r' || r >= 0x20 {
			continue
		}
		return fmt.Errorf("%s содержит недопустимый для XML 1.0 управляющий символ U+%04X", field, r)
	}
	return nil
}

func cleanNumber(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	if _, err := strconv.Atoi(value); err != nil {
		return fallback
	}
	return value
}

func normalizeContextInteger(value, field string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "0", nil
	}
	number, err := strconv.ParseInt(value, 10, 32)
	if err != nil || number < 0 {
		return "", fmt.Errorf("%s должен быть неотрицательным signed 32-bit целым", field)
	}
	return strconv.FormatInt(number, 10), nil
}

func normalizeBool(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "-1":
		return "true", nil
	case "", "false", "0":
		return "false", nil
	default:
		return "", fmt.Errorf("неизвестное булево значение %q", value)
	}
}

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

func resolvePOUName(requested, templateName string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return makePOUName(templateName), nil
	}
	if err := validatePOUName(requested); err != nil {
		return "", err
	}
	return requested, nil
}

func validatePOUName(value string) error {
	if value == "" {
		return fmt.Errorf("имя программного модуля POU не задано")
	}
	if len([]rune(value)) > 160 {
		return fmt.Errorf("имя программного модуля POU не должно быть длиннее 160 символов")
	}
	for index, r := range []rune(value) {
		if index == 0 {
			if unicode.IsLetter(r) || r == '_' {
				continue
			}
			return fmt.Errorf("имя программного модуля POU должно начинаться с буквы или подчёркивания")
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			continue
		}
		return fmt.Errorf("имя программного модуля POU содержит недопустимый символ %q", r)
	}
	return nil
}

func makePOUName(templateName string) string {
	value := "POU_" + templateName
	var builder strings.Builder
	lastUnderscore := false
	for _, r := range value {
		valid := unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
		if !valid {
			r = '_'
		}
		if r == '_' && lastUnderscore {
			continue
		}
		builder.WriteRune(r)
		lastUnderscore = r == '_'
	}
	result := strings.TrimRight(builder.String(), "_")
	if result == "" || result == "POU" {
		return "Generated_POU"
	}
	if len([]rune(result)) > 160 {
		result = string([]rune(result)[:160])
		result = strings.TrimRight(result, "_")
	}
	return result
}

func appendOnce(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func uniqueStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{})
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
