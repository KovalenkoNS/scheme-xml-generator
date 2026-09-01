package library

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Repository struct {
	directory string
	mu        sync.RWMutex
	libraries map[string]*LoadedLibrary
	templates map[string]*TemplateRef
	catalog   Catalog
}

type Catalog struct {
	Libraries []LibrarySummary  `json:"libraries"`
	Templates []TemplateSummary `json:"templates"`
	Errors    []LoadError       `json:"errors"`
}

type LibrarySummary struct {
	File          string `json:"file"`
	Version       string `json:"version"`
	TemplateCount int    `json:"templateCount"`
}

type LoadError struct {
	File    string `json:"file"`
	Message string `json:"message"`
}

type TemplateSummary struct {
	Key            string          `json:"key"`
	LibraryFile    string          `json:"libraryFile"`
	OwnerID        string          `json:"ownerId"`
	OwnerName      string          `json:"ownerName"`
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	Width          int             `json:"width"`
	Height         int             `json:"height"`
	PrimitiveCount int             `json:"primitiveCount"`
	BlockCount     int             `json:"blockCount"`
	LinkCount      int             `json:"linkCount"`
	GraphicCount   int             `json:"graphicCount"`
	CardCount      int             `json:"cardCount"`
	Warnings       []string        `json:"warnings"`
	Preview        TemplatePreview `json:"preview"`
}

type TemplatePreview struct {
	Width  int            `json:"width"`
	Height int            `json:"height"`
	Blocks []PreviewBlock `json:"blocks"`
	Lines  [][]Point      `json:"lines"`
}

type PreviewBlock struct {
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Label  string `json:"label"`
	Type   string `json:"type"`
}

func NewRepository(directory string) *Repository {
	return &Repository{directory: directory}
}

func (r *Repository) Refresh() (Catalog, error) {
	entries, err := os.ReadDir(r.directory)
	if err != nil {
		return Catalog{}, fmt.Errorf("прочитать каталог библиотек: %w", err)
	}

	loaded := make(map[string]*LoadedLibrary)
	references := make(map[string]*TemplateRef)
	catalog := Catalog{Libraries: []LibrarySummary{}, Templates: []TemplateSummary{}, Errors: []LoadError{}}

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
		count := 0
		walkObjectTypes(library.Document, func(owner *ObjectType) {
			for i := range owner.Templates.Items {
				template := &owner.Templates.Items[i]
				key := templateKey(entry.Name(), owner.ID, template.ID)
				ref := &TemplateRef{Key: key, Library: library, Owner: owner, Template: template}
				references[key] = ref
				catalog.Templates = append(catalog.Templates, summarizeTemplate(ref))
				count++
			}
		})
		catalog.Libraries = append(catalog.Libraries, LibrarySummary{File: entry.Name(), Version: library.Version, TemplateCount: count})
	}

	sort.Slice(catalog.Libraries, func(i, j int) bool { return catalog.Libraries[i].File < catalog.Libraries[j].File })
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

func (r *Repository) Catalog() Catalog {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return cloneCatalog(r.catalog)
}

func (r *Repository) Resolve(key string) (*TemplateRef, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ref, ok := r.templates[key]
	return ref, ok
}

func cloneCatalog(catalog Catalog) Catalog {
	data, _ := json.Marshal(catalog)
	var clone Catalog
	_ = json.Unmarshal(data, &clone)
	return clone
}

func loadLibrary(path, name string) (*LoadedLibrary, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	limited := io.LimitReader(file, 128<<20)
	decoder := xml.NewDecoder(limited)
	decoder.Strict = true
	document := &Document{}
	if err := decoder.Decode(document); err != nil {
		return nil, fmt.Errorf("некорректный XML библиотеки: %w", err)
	}
	library := &LoadedLibrary{
		FileName:       name,
		Path:           path,
		Version:        document.Version.Value,
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

func walkObjectTypes(document *Document, visit func(*ObjectType)) {
	var walk func([]ObjectType)
	walk = func(objects []ObjectType) {
		for i := range objects {
			object := &objects[i]
			visit(object)
			walk(object.Child.ObjectTypes)
		}
	}
	for i := range document.Sections {
		walk(document.Sections[i].Other.ObjectTypes)
	}
}

func templateKey(file, ownerID, templateID string) string {
	sum := sha256.Sum256([]byte(file + "\x00" + ownerID + "\x00" + templateID))
	return hex.EncodeToString(sum[:12])
}

func summarizeTemplate(ref *TemplateRef) TemplateSummary {
	template := ref.Template
	summary := TemplateSummary{
		Key:            ref.Key,
		LibraryFile:    ref.Library.FileName,
		OwnerID:        ref.Owner.ID,
		OwnerName:      ref.Owner.Name,
		ID:             template.ID,
		Name:           template.Name,
		Description:    strings.TrimSpace(template.Description),
		Width:          Int(template.Width, 0),
		Height:         Int(template.Height, 0),
		PrimitiveCount: len(template.Contents.Primitives),
		Warnings:       []string{},
	}
	if strings.HasPrefix(strings.ToUpper(summary.Description), "OLD") {
		summary.Warnings = append(summary.Warnings, "Шаблон помечен библиотекой как OLD/legacy.")
	}
	cards := make(map[string]struct{})
	cardIndex := make(map[string]ISAObject)
	for _, card := range ref.Owner.ISAObjects.Items {
		cardIndex[card.ID] = card
	}
	preview := TemplatePreview{Width: summary.Width, Height: summary.Height, Blocks: []PreviewBlock{}, Lines: [][]Point{}}
	for _, primitive := range template.Contents.Primitives {
		switch {
		case IsLinkType(primitive.ObjectType):
			summary.LinkCount++
			params := ParseParams(primitive.Params)
			if points, err := ParsePoints(params["PL"]); err == nil {
				preview.Lines = append(preview.Lines, points)
			}
		case IsGraphicType(primitive.ObjectType):
			summary.GraphicCount++
		case IsSupportedBlockType(primitive.ObjectType):
			summary.BlockCount++
			params := ParseParams(primitive.Params)
			label := strings.TrimSpace(primitive.TypeName)
			if label == "" {
				label = strings.TrimSpace(params["TEXT"])
			}
			if label == "" && primitive.CardID != "" && primitive.CardID != "0" {
				if card, ok := cardIndex[primitive.CardID]; ok {
					label = card.EffectivePrefix()
				}
			}
			if label == "" && primitive.ObjectType == "35" {
				switch strings.TrimSpace(primitive.ISAObjectID) {
				case "-3":
					label = "+"
				case "-4":
					label = "/"
				case "-33":
					label = "OR"
				}
			}
			if label == "" {
				label = "T" + primitive.ObjectType
			}
			preview.Blocks = append(preview.Blocks, PreviewBlock{
				X: Int(primitive.X, 0), Y: Int(primitive.Y, 0), Width: Int(primitive.Width, 0), Height: Int(primitive.Height, 0), Label: label, Type: primitive.ObjectType,
			})
		default:
			summary.Warnings = append(summary.Warnings, "Неизвестный GROBJTYPE="+primitive.ObjectType)
		}
		if primitive.CardID != "" && primitive.CardID != "0" {
			cards[primitive.CardID] = struct{}{}
			if _, ok := cardIndex[primitive.CardID]; !ok {
				summary.Warnings = append(summary.Warnings, "CARDID "+primitive.CardID+" отсутствует в ISAOBJLIST")
			}
		}
	}
	summary.CardCount = len(cards)
	preview.Width = max(preview.Width, 1)
	preview.Height = max(preview.Height, 1)
	summary.Preview = preview
	return summary
}
