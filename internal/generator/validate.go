package generator

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

func validateGeneratedXML(data []byte, primitiveCount int, blocks []outputBlock, links []outputLink, graphics []outputPrimitive, fontCount int, allowPhysicalRetain bool) error {
	if len(blocks)+len(links)+len(graphics) != primitiveCount {
		return fmt.Errorf("число созданных объектов не совпадает с числом grprim")
	}
	ids := make(map[string]struct{}, len(blocks)+len(graphics))
	blockIDs := make(map[string]struct{}, len(blocks))
	for _, block := range blocks {
		if _, exists := ids[block.T11ID]; exists {
			return fmt.Errorf("повторный T11ID=%s", block.T11ID)
		}
		ids[block.T11ID] = struct{}{}
		blockIDs[block.T11ID] = struct{}{}
	}
	for _, primitive := range graphics {
		if _, exists := ids[primitive.SourceT11ID]; exists {
			return fmt.Errorf("повторный графический T11ID=%s", primitive.SourceT11ID)
		}
		ids[primitive.SourceT11ID] = struct{}{}
	}
	for _, link := range links {
		for _, endpoint := range []string{link.FirstPoint.Value, link.LastPoint.Last} {
			id := strings.SplitN(endpoint, "|", 2)[0]
			if _, ok := blockIDs[id]; !ok {
				return fmt.Errorf("endpoint ссылается на отсутствующий T11ID=%s", id)
			}
		}
	}
	if bytes.Contains(data, []byte("{{")) {
		return fmt.Errorf("результат содержит незаменённый placeholder")
	}
	if err := validateSCADADialect(data, fontCount, allowPhysicalRetain); err != nil {
		return err
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	decoder.Strict = true
	for {
		_, err := decoder.Token()
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			break
		}
		return fmt.Errorf("повторная проверка XML: %w", err)
	}
	return nil
}

func validateGeneratedDocumentXML(data []byte, primitiveCount int, document outputDocument) error {
	return validateGeneratedDocumentForProfile(data, primitiveCount, document, false)
}

// nativeAO permits the exact AN_v1 card/block form observed in the AO export.
// Ordinary library generation retains its existing, stricter policy.
func validateGeneratedDocumentForProfile(data []byte, primitiveCount int, document outputDocument, nativeAO bool) error {
	if len(document.POUS.Items) == 0 {
		return fmt.Errorf("созданный документ не содержит POU")
	}
	blocks := make([]outputBlock, 0)
	links := make([]outputLink, 0)
	graphics := make([]outputPrimitive, 0)
	pouIDs := make(map[string]struct{}, len(document.POUS.Items))
	pouNames := make(map[string]struct{}, len(document.POUS.Items))
	pouNumbers := make(map[string]struct{}, len(document.POUS.Items))
	for _, pou := range document.POUS.Items {
		if _, exists := pouIDs[pou.ID]; exists {
			return fmt.Errorf("повторный POU ID=%s", pou.ID)
		}
		pouIDs[pou.ID] = struct{}{}
		nameKey := strings.ToUpper(pou.Name)
		if _, exists := pouNames[nameKey]; exists {
			return fmt.Errorf("повторное имя POU=%s", pou.Name)
		}
		pouNames[nameKey] = struct{}{}
		if _, exists := pouNumbers[pou.Number]; exists {
			return fmt.Errorf("повторный POUNum=%s", pou.Number)
		}
		pouNumbers[pou.Number] = struct{}{}

		localBlockIDs := make(map[string]struct{}, len(pou.ISAGraf.Blocks.Items))
		for _, block := range pou.ISAGraf.Blocks.Items {
			localBlockIDs[block.T11ID] = struct{}{}
		}
		for _, link := range pou.ISAGraf.Links.Items {
			for _, endpoint := range []string{link.FirstPoint.Value, link.LastPoint.Last} {
				id := strings.SplitN(endpoint, "|", 2)[0]
				if _, ok := localBlockIDs[id]; !ok {
					return fmt.Errorf("POU %s: endpoint ссылается на T11ID=%s из другого POU или на отсутствующий блок", pou.Name, id)
				}
			}
		}
		blocks = append(blocks, pou.ISAGraf.Blocks.Items...)
		links = append(links, pou.ISAGraf.Links.Items...)
		graphics = append(graphics, pou.Graphics.Items...)
	}

	cardIDs := make(map[string]struct{}, len(document.ISACards.Items))
	cardInfos := make(map[string]struct{}, len(document.ISACards.Items))
	for _, card := range document.ISACards.Items {
		if _, exists := cardIDs[card.ID]; exists {
			return fmt.Errorf("повторный cardId=%s", card.ID)
		}
		cardIDs[card.ID] = struct{}{}
		infoKey := strings.ToUpper(strings.TrimSpace(card.Info))
		if _, exists := cardInfos[infoKey]; exists {
			return fmt.Errorf("повторный Card.Info=%s", card.Info)
		}
		cardInfos[infoKey] = struct{}{}
	}
	for _, block := range blocks {
		if block.Params.CardID == "" || block.Params.CardID == "0" {
			continue
		}
		if _, exists := cardIDs[block.Params.CardID]; !exists {
			return fmt.Errorf("Block T11ID=%s ссылается на отсутствующий cardId=%s", block.T11ID, block.Params.CardID)
		}
	}
	cardRetain := make(map[string]string, len(document.ISACards.Items))
	for _, card := range document.ISACards.Items {
		cardRetain[card.ID] = card.IsRetain
	}
	negativeUsage := make(map[string]int)
	for _, block := range blocks {
		if block.Params.CardID == "" || block.Params.CardID == "0" {
			continue
		}
		if cardRetain[block.Params.CardID] == "-1" {
			isExternalChannel := block.ObjectType == "38"
			isDigitalModule := block.ObjectType == "37" && block.Params.ISAObjectID == "1933"
			isNativeAO := nativeAO && block.ObjectType == "37" && block.Params.ISAObjectID == "888" && block.Params.CI == "7" && block.Params.CO == "2" && block.Params.Initial != nil && *block.Params.Initial == ",,,100.0"
			if !isExternalChannel && !isDigitalModule && !isNativeAO {
				return fmt.Errorf("физическая карточка cardId=%s используется неподдерживаемым блоком GROBJTYPE=%s IsaObjId=%s", block.Params.CardID, block.ObjectType, block.Params.ISAObjectID)
			}
			negativeUsage[block.Params.CardID]++
		} else if block.ObjectType == "38" {
			return fmt.Errorf("блок GROBJTYPE=38 T11ID=%s ссылается на карточку без IsRetain=-1", block.T11ID)
		}
	}
	for _, card := range document.ISACards.Items {
		if card.IsRetain == "-1" && negativeUsage[card.ID] == 0 {
			return fmt.Errorf("физическая карточка cardId=%s не используется GROBJTYPE=38", card.ID)
		}
	}
	fontCount := 0
	if document.FontStyles != nil {
		fontCount = len(document.FontStyles.Items)
	}
	if err := validateGeneratedXML(data, primitiveCount, blocks, links, graphics, fontCount, true); err != nil {
		return err
	}
	if count := bytes.Count(data, []byte("<Gotos/>")); count != len(document.POUS.Items) {
		return fmt.Errorf("число Gotos=%d не совпадает с числом POU=%d", count, len(document.POUS.Items))
	}
	if count := bytes.Count(data, []byte("<Links")); count != len(document.POUS.Items) {
		return fmt.Errorf("число Links=%d не совпадает с числом POU=%d", count, len(document.POUS.Items))
	}
	return nil
}

func validateSCADADialect(data []byte, fontCount int, allowPhysicalRetain bool) error {
	wantPrefix := append(append([]byte{}, utf8BOM...), []byte(xml.Header)...)
	if !bytes.HasPrefix(data, wantPrefix) {
		return fmt.Errorf("SCADA XML должен начинаться с UTF-8 BOM и XML declaration")
	}
	if bytes.Contains(data, []byte("\n  <")) || bytes.Contains(data, []byte("\r\n  <")) {
		return fmt.Errorf("SCADA XML содержит форматирующие межэлементные пробелы")
	}
	if name := expandedEmptyElement(data); name != "" {
		return fmt.Errorf("пустой элемент %s должен быть self-closing", name)
	}
	if fontCount == 0 && bytes.Contains(data, []byte("<FONTSTYLES")) {
		return fmt.Errorf("пустая optional-секция FONTSTYLES должна отсутствовать")
	}
	if fontCount > 0 && !bytes.Contains(data, []byte("<FONTSTYLES>")) {
		return fmt.Errorf("FONTSTYLES отсутствует при наличии стилей")
	}
	if !bytes.Contains(data, []byte("<Gotos/>")) {
		return fmt.Errorf("обязательный пустой элемент Gotos должен присутствовать")
	}

	booleanAttributes := map[string]map[string]struct{}{
		"isCut":     {"false": {}, "true": {}},
		"isFFB":     {"false": {}, "true": {}},
		"Commented": {"false": {}, "true": {}},
		"Negative":  {"false": {}, "true": {}},
		"asPointer": {"false": {}, "true": {}},
		"UserEdit":  {"false": {}, "true": {}},
		"isFBD":     {"0": {}, "1": {}},
		"Enabled":   {"0": {}, "1": {}},
		"IsRetain":  {"0": {}, "1": {}},
	}
	if allowPhysicalRetain {
		booleanAttributes["IsRetain"]["-1"] = struct{}{}
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	depth := 0
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("проверить SCADA dialect: %w", err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			depth++
			for _, attribute := range value.Attr {
				allowed, isBoolean := booleanAttributes[attribute.Name.Local]
				if !isBoolean {
					continue
				}
				if _, ok := allowed[attribute.Value]; !ok {
					return fmt.Errorf("%s/@%s содержит несовместимое булево значение %q", value.Name.Local, attribute.Name.Local, attribute.Value)
				}
			}
		case xml.EndElement:
			depth--
		case xml.CharData:
			if depth > 0 && len(value) > 0 && strings.TrimSpace(string(value)) == "" {
				return fmt.Errorf("SCADA XML содержит форматирующий text-node внутри документа")
			}
		}
	}
	return nil
}

func expandedEmptyElement(data []byte) string {
	for offset := 0; offset < len(data); {
		relative := bytes.IndexByte(data[offset:], '<')
		if relative < 0 {
			return ""
		}
		start := offset + relative
		if start+1 >= len(data) || data[start+1] == '/' || data[start+1] == '?' || data[start+1] == '!' {
			offset = start + 1
			continue
		}
		endRelative := bytes.IndexByte(data[start:], '>')
		if endRelative < 0 {
			return ""
		}
		end := start + endRelative
		if end > start && data[end-1] == '/' {
			offset = end + 1
			continue
		}
		nameEnd := start + 1
		for nameEnd < end && data[nameEnd] != ' ' && data[nameEnd] != '\t' && data[nameEnd] != '\r' && data[nameEnd] != '\n' {
			nameEnd++
		}
		if nameEnd == start+1 {
			offset = end + 1
			continue
		}
		name := data[start+1 : nameEnd]
		closing := append(append([]byte("</"), name...), []byte(">")...)
		if bytes.HasPrefix(data[end+1:], closing) {
			return string(name)
		}
		offset = end + 1
	}
	return ""
}
