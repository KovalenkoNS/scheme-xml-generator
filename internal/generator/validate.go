package generator

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

func validateGeneratedXML(data []byte, primitiveCount int, blocks []outputBlock, links []outputLink, graphics []outputPrimitive, fontCount int) error {
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
	if err := validateSCADADialect(data, fontCount); err != nil {
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

func validateSCADADialect(data []byte, fontCount int) error {
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
