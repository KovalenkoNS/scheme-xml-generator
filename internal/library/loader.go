// Строгое чтение XML-библиотеки и построение стилей/сигнатур загруженного документа.
package library

import (
	"fmt"

	"io"

	"os"

	"strings"

	"encoding/xml"
)

// loadLibrary читает и проверяет XML-библиотеку перед добавлением в Repository.
// Ограничивает размер, отклоняет лишний XML-хвост, возвращает документ со стилями/сигнатурами и предупреждениями C0.
func loadLibrary(path, name string) (*LoadedLibrary, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("прочитать размер библиотеки: %w", err)
	}
	if info.Size() > MaxLibraryBytes {
		return nil, fmt.Errorf("размер библиотеки %d МБ превышает допустимые 512 МБ", info.Size()>>20)
	}
	filtered := &xmlControlFilter{reader: io.LimitReader(file, MaxLibraryBytes+1), counts: make(map[byte]int)}
	decoder := xml.NewDecoder(filtered)
	decoder.Strict = true
	document := &Document{}
	if err := decoder.Decode(document); err != nil {
		return nil, fmt.Errorf("некорректный XML библиотеки: %w", err)
	}
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("некорректное окончание XML библиотеки: %w", err)
		}
		switch value := token.(type) {
		case xml.CharData:
			if strings.TrimSpace(string(value)) != "" {
				return nil, fmt.Errorf("после XML библиотеки обнаружены лишние данные")
			}
		case xml.Comment, xml.ProcInst:
		default:
			return nil, fmt.Errorf("после XML библиотеки обнаружены лишние данные")
		}
	}
	library := &LoadedLibrary{
		FileName:       name,
		Path:           path,
		Version:        document.Version.Value,
		Warnings:       filtered.warnings(),
		Document:       document,
		FontStyles:     make(map[string]FontStyle),
		TypeSignatures: make(map[string]Signature),
	}
	for _, style := range document.RootFontStyles {
		library.FontStyles[style.ID] = style
	}
	walkObjectTypes(document, func(owner *ObjectType) {
		for _, template := range owner.Templates.Items {
			for _, primitive := range template.Contents.Primitives {
				if primitive.ObjectType != "36" && primitive.ObjectType != "37" {
					continue
				}
				params := ParseParams(primitive.Params)
				key := SignatureKey(primitive.ISAObjectID, primitive.TypeName)
				current := library.TypeSignatures[key]
				ci, co := Int(params["CI"], 0), Int(params["CO"], 0)
				if ci > current.CI {
					current.CI = ci
				}
				if co > current.CO {
					current.CO = co
				}
				library.TypeSignatures[key] = current
			}
		}
	})
	return library, nil
}
