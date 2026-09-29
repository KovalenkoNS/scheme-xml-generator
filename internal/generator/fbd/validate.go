// Проверка сформированного FBD сопоставляет XML с ожидаемыми блоками, связями, карточками и пространствами ID.
package fbd

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"scheme-xml-generator/internal/generator/xmlcodec"
	"scheme-xml-generator/internal/generator/xmlmodel"
	"strings"
)

// validateGeneratedXML Проверяет ссылки, количество примитивов и диалект собранного FBD XML.
// Сверяет сериализованные данные с ожидаемыми блоками/связями; импорт SCADA не выполняет.
func validateGeneratedXML(data []byte, primitiveCount int, blocks []xmlmodel.OutputBlock, links []xmlmodel.OutputLink, graphics []xmlmodel.OutputPrimitive, fontCount int, allowPhysicalRetain bool) error {
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
	if err := xmlcodec.ValidateSCADADialect(data, fontCount, allowPhysicalRetain); err != nil {
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

// validateGeneratedDocumentXML Запускает структурную проверку объединённого библиотечного FBD-документа.
// Использует ожидаемую модель и количество примитивов, не заявляет выполнение SCADA.
func validateGeneratedDocumentXML(data []byte, primitiveCount int, document xmlmodel.OutputDocument) error {
	return validateGeneratedDocumentForProfile(data, primitiveCount, document, false)
}

// nativeAO permits the exact AN_v1 card/block form observed in the AO export.
// Ordinary library generation retains its existing, stricter policy.
func validateGeneratedDocumentForProfile(data []byte, primitiveCount int, document xmlmodel.OutputDocument, nativeAO bool) error {
	if len(document.POUS.Items) == 0 {
		return fmt.Errorf("созданный документ не содержит POU")
	}
	blocks := make([]xmlmodel.OutputBlock, 0)
	links := make([]xmlmodel.OutputLink, 0)
	graphics := make([]xmlmodel.OutputPrimitive, 0)
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
