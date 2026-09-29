// Single-template FBD types stage: library-backed output preparation and assembly.
package fbd

import (
	"fmt"

	"scheme-xml-generator/internal/generator/xmlmodel"
	"scheme-xml-generator/internal/library"
	"sort"

	"strings"
)

// buildISAObjects Разрешает определения ISA-типов примитивов выбранного шаблона в библиотеке.
// Возвращает записи типов для XML либо ошибку отсутствующего определения.
func buildISAObjects(ref *library.TemplateRef) ([]xmlmodel.OutputISAObject, error) {
	objects := make(map[string]xmlmodel.OutputISAObject)
	for _, primitive := range ref.Template.Contents.Primitives {
		if !library.IsSupportedBlockType(primitive.ObjectType) || library.Int(primitive.ISAObjectID, 0) <= 0 {
			continue
		}
		id := strings.TrimSpace(primitive.ISAObjectID)
		record := xmlmodel.OutputISAObject{ID: id, Info: primitive.TypeName, LibraryName: primitive.LibraryName}
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
	result := make([]xmlmodel.OutputISAObject, 0, len(ids))
	for _, id := range ids {
		result = append(result, objects[id])
	}
	return result, nil
}
