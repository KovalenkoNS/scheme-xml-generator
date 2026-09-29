// Лексическая проверка XML контролирует self-closing элементы, булевы атрибуты и отсутствие форматирующих text nodes.
package xmlcodec

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ValidateSCADADialect Проверяет BOM, пустые элементы, булевы атрибуты и обязательные секции SCADA XML.
// Это лексическая проверка выхода сериализатора, не импорт или исполнение в SCADA.
func ValidateSCADADialect(data []byte, fontCount int, allowPhysicalRetain bool) error {
	wantPrefix := append(append([]byte{}, Utf8BOM...), []byte(xml.Header)...)
	if !bytes.HasPrefix(data, wantPrefix) {
		return fmt.Errorf("SCADA XML должен начинаться с UTF-8 BOM и XML declaration")
	}
	if bytes.Contains(data, []byte("\n  <")) || bytes.Contains(data, []byte("\r\n  <")) {
		return fmt.Errorf("SCADA XML содержит форматирующие межэлементные пробелы")
	}
	if name := ExpandedEmptyElement(data); name != "" {
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

// ExpandedEmptyElement Находит пустой XML-элемент, ошибочно записанный парой тегов.
// Возвращает его имя для сообщения валидатора SCADA-диалекта либо пустую строку.
func ExpandedEmptyElement(data []byte) string {
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
