// Полное обновление индекса типов/шаблонов из библиотеки с атомарной заменой снимка.
package library

import (
	"fmt"
	"path/filepath"

	"os"

	"sort"
	"strings"
)

// Refresh перечитывает подключённые XML-библиотеки по запросу запуска или интерфейса.
// Сериализует обновления/импорт общей блокировкой и возвращает независимый снимок каталога.
func (r *Repository) Refresh() (Catalog, error) {
	r.refreshMu.Lock()
	defer r.refreshMu.Unlock()
	return r.refresh()
}

// refresh строит новый индекс типов и шаблонов всех XML-файлов каталога.
// Собирает ошибки отдельных библиотек и атомарно заменяет доступный снимок под блокировкой Repository.
func (r *Repository) refresh() (Catalog, error) {
	entries, err := os.ReadDir(r.directory)
	if err != nil {
		return Catalog{}, fmt.Errorf("прочитать каталог библиотек: %w", err)
	}

	loaded := make(map[string]*LoadedLibrary)
	references := make(map[string]*TemplateRef)
	catalog := Catalog{Libraries: []LibrarySummary{}, Templates: []TemplateSummary{}, Types: []ObjectTypeSummary{}, Errors: []LoadError{}}

	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".xml") {
			continue
		}
		fullPath := filepath.Join(r.directory, entry.Name())
		library, loadErr := loadLibrary(fullPath, entry.Name())
		if loadErr != nil {
			catalog.Errors = append(catalog.Errors, LoadError{File: entry.Name(), Message: loadErr.Error()})
			continue
		}
		loaded[entry.Name()] = library
		count, supportedCount, typeCount := 0, 0, 0
		walkObjectTypes(library.Document, func(owner *ObjectType) {
			typeCount++
			objectType := ObjectTypeSummary{LibraryFile: entry.Name(), ID: owner.ID, Name: owner.Name, TemplateCount: len(owner.Templates.Items), Fields: []TypeFieldSummary{}}
			for _, field := range owner.ISAObjects.Items {
				objectType.Fields = append(objectType.Fields, TypeFieldSummary{ID: field.ID, Name: field.EffectivePrefix(), TypeName: field.TypeName, LibraryName: field.LibraryName, Kind: field.Kind})
			}
			catalog.Types = append(catalog.Types, objectType)
			for i := range owner.Templates.Items {
				template := &owner.Templates.Items[i]
				key := templateKey(entry.Name(), owner.ID, template.ID)
				ref := &TemplateRef{Key: key, Library: library, Owner: owner, Template: template}
				references[key] = ref
				summary := summarizeTemplate(ref)
				catalog.Templates = append(catalog.Templates, summary)
				count++
				if summary.Supported {
					supportedCount++
				}
			}
		})
		catalog.Libraries = append(catalog.Libraries, LibrarySummary{File: entry.Name(), Version: library.Version, TemplateCount: count, SupportedTemplateCount: supportedCount, TypeCount: typeCount, Warnings: append([]string(nil), library.Warnings...)})
	}

	sort.Slice(catalog.Libraries, func(i, j int) bool { return catalog.Libraries[i].File < catalog.Libraries[j].File })
	sort.Slice(catalog.Types, func(i, j int) bool {
		if catalog.Types[i].LibraryFile != catalog.Types[j].LibraryFile {
			return catalog.Types[i].LibraryFile < catalog.Types[j].LibraryFile
		}
		return catalog.Types[i].ID < catalog.Types[j].ID
	})
	sort.Slice(catalog.Templates, func(i, j int) bool {
		if catalog.Templates[i].LibraryFile != catalog.Templates[j].LibraryFile {
			return catalog.Templates[i].LibraryFile < catalog.Templates[j].LibraryFile
		}
		return Int(catalog.Templates[i].ID, 0) < Int(catalog.Templates[j].ID, 0)
	})

	r.mu.Lock()
	r.libraries = loaded
	r.templates = references
	r.catalog = catalog
	r.mu.Unlock()
	return cloneCatalog(catalog), nil
}
