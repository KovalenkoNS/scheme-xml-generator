package generator

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// serializeSCADA writes the lexical XML dialect produced by the legacy SCADA
// exporter. Its importer is sensitive to formatting text nodes and to the
// distinction between an empty value and a self-closing element.
func serializeSCADA(document outputDocument) ([]byte, error) {
	return serializeSCADAValue(document)
}

// serializeSCADAValue also supports native ST documents, which intentionally
// have no graphical object or card sections.
func serializeSCADAValue(document any) ([]byte, error) {
	marshaled, err := xml.Marshal(document)
	if err != nil {
		return nil, err
	}

	decoder := xml.NewDecoder(bytes.NewReader(marshaled))
	var result bytes.Buffer
	result.Write(utf8BOM)
	result.WriteString(xml.Header)

	var pending *xml.StartElement
	stack := make([]xml.Name, 0, 16)
	flushPending := func() error {
		if pending == nil {
			return nil
		}
		if err := writeStartElement(&result, *pending, false); err != nil {
			return err
		}
		pending = nil
		return nil
	}

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch value := xml.CopyToken(token).(type) {
		case xml.StartElement:
			if err := flushPending(); err != nil {
				return nil, err
			}
			pending = &value
			stack = append(stack, value.Name)
		case xml.EndElement:
			if len(stack) == 0 || stack[len(stack)-1] != value.Name {
				return nil, fmt.Errorf("несбалансированный закрывающий элемент %s", value.Name.Local)
			}
			stack = stack[:len(stack)-1]
			if pending != nil && pending.Name == value.Name {
				if err := writeStartElement(&result, *pending, true); err != nil {
					return nil, err
				}
				pending = nil
				continue
			}
			if err := flushPending(); err != nil {
				return nil, err
			}
			result.WriteString("</")
			result.WriteString(value.Name.Local)
			result.WriteByte('>')
		case xml.CharData:
			if err := flushPending(); err != nil {
				return nil, err
			}
			if err := writeSCADAText(&result, string(value)); err != nil {
				return nil, err
			}
		case xml.Comment:
			if err := flushPending(); err != nil {
				return nil, err
			}
			result.WriteString("<!--")
			result.Write(value)
			result.WriteString("-->")
		default:
			return nil, fmt.Errorf("неподдерживаемый XML token %T", token)
		}
	}
	if err := flushPending(); err != nil {
		return nil, err
	}
	if len(stack) != 0 {
		return nil, fmt.Errorf("незакрытый XML element %s", stack[len(stack)-1].Local)
	}
	result.WriteByte('\n')
	return result.Bytes(), nil
}

func writeStartElement(buffer *bytes.Buffer, element xml.StartElement, selfClosing bool) error {
	buffer.WriteByte('<')
	buffer.WriteString(element.Name.Local)
	for _, attribute := range element.Attr {
		buffer.WriteByte(' ')
		buffer.WriteString(attribute.Name.Local)
		buffer.WriteString("=\"")
		if err := writeSCADAAttribute(buffer, attribute.Value); err != nil {
			return fmt.Errorf("attribute %s: %w", attribute.Name.Local, err)
		}
		buffer.WriteByte('"')
	}
	if selfClosing {
		buffer.WriteString("/>")
	} else {
		buffer.WriteByte('>')
	}
	return nil
}

func writeSCADAAttribute(buffer *bytes.Buffer, value string) error {
	for len(value) > 0 {
		r, size := utf8.DecodeRuneInString(value)
		if r == utf8.RuneError && size == 1 {
			return fmt.Errorf("неверная UTF-8 последовательность")
		}
		value = value[size:]
		switch r {
		case '&':
			buffer.WriteString("&amp;")
		case '<':
			buffer.WriteString("&lt;")
		case '>':
			buffer.WriteString("&gt;")
		case '"':
			buffer.WriteString("&quot;")
		case '\t':
			buffer.WriteString("&#x9;")
		case '\n':
			buffer.WriteString("&#xA;")
		case '\r':
			buffer.WriteString("&#xD;")
		default:
			if !validXMLRune(r) {
				return fmt.Errorf("недопустимый для XML 1.0 символ U+%04X", r)
			}
			buffer.WriteRune(r)
		}
	}
	return nil
}

func writeSCADAText(buffer *bytes.Buffer, value string) error {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	for len(value) > 0 {
		r, size := utf8.DecodeRuneInString(value)
		if r == utf8.RuneError && size == 1 {
			return fmt.Errorf("неверная UTF-8 последовательность")
		}
		value = value[size:]
		switch r {
		case '&':
			buffer.WriteString("&amp;")
		case '<':
			buffer.WriteString("&lt;")
		case '>':
			buffer.WriteString("&gt;")
		case '\n':
			buffer.WriteString("\r\n")
		default:
			if !validXMLRune(r) {
				return fmt.Errorf("недопустимый для XML 1.0 символ U+%04X", r)
			}
			buffer.WriteRune(r)
		}
	}
	return nil
}

func validXMLRune(r rune) bool {
	return r == '\t' || r == '\n' || r == '\r' || r >= 0x20 && r <= 0xD7FF || r >= 0xE000 && r <= 0xFFFD || r >= 0x10000 && r <= 0x10FFFF
}
