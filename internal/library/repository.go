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
	File                   string   `json:"file"`
	Version                string   `json:"version"`
	TemplateCount          int      `json:"templateCount"`
	SupportedTemplateCount int      `json:"supportedTemplateCount"`
	Warnings               []string `json:"warnings"`
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
	Supported      bool            `json:"supported"`
	Warnings       []string        `json:"warnings"`
	IOCapabilities []string        `json:"ioCapabilities"`
	Layout         LayoutBounds    `json:"layout"`
	Preview        TemplatePreview `json:"preview"`
}

type ioEndpoint struct {
	id   string
	port string
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
		count, supportedCount := 0, 0
		walkObjectTypes(library.Document, func(owner *ObjectType) {
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
		catalog.Libraries = append(catalog.Libraries, LibrarySummary{File: entry.Name(), Version: library.Version, TemplateCount: count, SupportedTemplateCount: supportedCount, Warnings: append([]string(nil), library.Warnings...)})
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

// ResolveMany resolves one immutable catalog snapshot under a single read
// lock. Repeated keys are returned once in the map, which is convenient for
// documents containing many instances of the same template.
func (r *Repository) ResolveMany(keys []string) (map[string]*TemplateRef, []string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	resolved := make(map[string]*TemplateRef, len(keys))
	missing := make([]string, 0)
	seenMissing := make(map[string]struct{})
	for _, key := range keys {
		if _, exists := resolved[key]; exists {
			continue
		}
		if ref, ok := r.templates[key]; ok {
			resolved[key] = ref
			continue
		}
		if _, exists := seenMissing[key]; !exists {
			seenMissing[key] = struct{}{}
			missing = append(missing, key)
		}
	}
	return resolved, missing
}

// TemplateCompatibility applies the same structural checks that are exposed
// in the catalog. The server uses it before reserving persistent ID ranges.
func TemplateCompatibility(ref *TemplateRef) (bool, []string) {
	if ref == nil || ref.Template == nil || ref.Owner == nil || ref.Library == nil {
		return false, []string{"Шаблон не полностью загружен."}
	}
	summary := summarizeTemplate(ref)
	return summary.Supported, append([]string(nil), summary.Warnings...)
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
	const maxLibrarySize int64 = 512 << 20
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("прочитать размер библиотеки: %w", err)
	}
	if info.Size() > maxLibrarySize {
		return nil, fmt.Errorf("размер библиотеки %d МБ превышает допустимые 512 МБ", info.Size()>>20)
	}
	filtered := &xmlControlFilter{reader: io.LimitReader(file, maxLibrarySize+1), counts: make(map[byte]int)}
	decoder := xml.NewDecoder(filtered)
	decoder.Strict = true
	document := &Document{}
	if err := decoder.Decode(document); err != nil {
		return nil, fmt.Errorf("некорректный XML библиотеки: %w", err)
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

// xmlControlFilter keeps vendor library exports readable without modifying
// the source file. Some SCADA exports contain isolated C0 bytes that XML 1.0
// forbids. Replacing only those bytes with a space preserves element framing
// and the surrounding human-readable parameter value.
type xmlControlFilter struct {
	reader io.Reader
	counts map[byte]int
}

func (f *xmlControlFilter) Read(buffer []byte) (int, error) {
	count, err := f.reader.Read(buffer)
	for index := 0; index < count; index++ {
		value := buffer[index]
		if value < 0x20 && value != '\t' && value != '\n' && value != '\r' {
			f.counts[value]++
			buffer[index] = ' '
		}
	}
	return count, err
}

func (f *xmlControlFilter) warnings() []string {
	if len(f.counts) == 0 {
		return []string{}
	}
	codes := make([]int, 0, len(f.counts))
	total := 0
	for value, count := range f.counts {
		codes = append(codes, int(value))
		total += count
	}
	sort.Ints(codes)
	labels := make([]string, 0, len(codes))
	for _, code := range codes {
		labels = append(labels, fmt.Sprintf("U+%04X×%d", code, f.counts[byte(code)]))
	}
	return []string{fmt.Sprintf("При чтении заменены пробелами недопустимые XML 1.0 управляющие символы: %s (всего %d). Исходный файл не изменён.", strings.Join(labels, ", "), total)}
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
		Supported:      true,
		Warnings:       []string{},
		IOCapabilities: ioCapabilities(template),
		Layout:         TemplateLayoutBounds(template),
	}
	if strings.HasPrefix(strings.ToUpper(summary.Description), "OLD") {
		summary.Warnings = append(summary.Warnings, "Шаблон помечен библиотекой как OLD/legacy.")
	}
	cards := make(map[string]struct{})
	cardIndex := make(map[string]ISAObject)
	for _, card := range ref.Owner.ISAObjects.Items {
		cardIndex[strings.TrimSpace(card.ID)] = card
	}
	blockIDs := make(map[string]struct{})
	primitiveIDs := make(map[string]struct{})
	cardOwners := make(map[string]bool)
	cardDependents := make(map[string]bool)
	isaDefinitions := make(map[string]string)
	for _, primitive := range template.Contents.Primitives {
		if IsSupportedBlockType(primitive.ObjectType) {
			blockIDs[strings.TrimSpace(primitive.ID)] = struct{}{}
		}
	}
	preview := TemplatePreview{Width: summary.Width, Height: summary.Height, Blocks: []PreviewBlock{}, Lines: [][]Point{}}
	for _, primitive := range template.Contents.Primitives {
		primitiveID := strings.TrimSpace(primitive.ID)
		if primitiveID == "" {
			summary.Supported = false
			summary.Warnings = appendStringOnce(summary.Warnings, "grprim содержит пустой ID")
		} else if _, exists := primitiveIDs[primitiveID]; exists {
			summary.Supported = false
			summary.Warnings = appendStringOnce(summary.Warnings, "Повторный grprim ID="+primitiveID)
		} else {
			primitiveIDs[primitiveID] = struct{}{}
		}
		switch {
		case IsLinkType(primitive.ObjectType):
			summary.LinkCount++
			params := ParseParams(primitive.Params)
			if points, err := ParsePoints(params["PL"]); err == nil {
				preview.Lines = append(preview.Lines, points)
			} else {
				summary.Supported = false
				summary.Warnings = appendStringOnce(summary.Warnings, "Связь содержит некорректный PointList")
			}
			for _, key := range []string{"FP", "LP"} {
				parts := strings.SplitN(strings.TrimSpace(params[key]), "|", 4)
				if len(parts) != 4 {
					summary.Supported = false
					summary.Warnings = appendStringOnce(summary.Warnings, key+" связи имеет некорректный формат")
					continue
				}
				if _, ok := blockIDs[strings.TrimSpace(parts[0])]; !ok {
					summary.Supported = false
					summary.Warnings = appendStringOnce(summary.Warnings, key+" связи ссылается на блок вне шаблона")
				}
				switch strings.ToLower(strings.TrimSpace(parts[1])) {
				case "0", "false", "-1", "true":
				default:
					summary.Supported = false
					summary.Warnings = appendStringOnce(summary.Warnings, key+" связи содержит неизвестное направление")
				}
			}
		case IsGraphicType(primitive.ObjectType):
			summary.GraphicCount++
			params := ParseParams(primitive.Params)
			if rawPoints := strings.TrimSpace(params["PL"]); rawPoints != "" {
				if _, err := ParsePoints(rawPoints); err != nil {
					summary.Supported = false
					summary.Warnings = appendStringOnce(summary.Warnings, "Графический примитив содержит некорректный PointList")
				}
			}
		case IsSupportedBlockType(primitive.ObjectType):
			summary.BlockCount++
			params := ParseParams(primitive.Params)
			switch strings.ToLower(strings.TrimSpace(params["COMMENT"])) {
			case "", "false", "0", "true", "1", "-1":
			default:
				summary.Supported = false
				summary.Warnings = appendStringOnce(summary.Warnings, "Block ID="+primitiveID+" содержит неизвестное Boolean COMMENT")
			}
			objectID := strings.TrimSpace(primitive.ISAObjectID)
			if Int(objectID, 0) > 0 {
				definition := strings.TrimSpace(primitive.TypeName) + "\x00" + strings.TrimSpace(primitive.LibraryName)
				if previous, exists := isaDefinitions[objectID]; exists && previous != definition {
					summary.Supported = false
					summary.Warnings = appendStringOnce(summary.Warnings, "OBJMSID "+objectID+" имеет противоречивые определения")
				} else {
					isaDefinitions[objectID] = definition
				}
			}
			cardID := strings.TrimSpace(primitive.CardID)
			if cardID != "" && cardID != "0" {
				if strings.TrimSpace(params["TEXT"]) == "" {
					cardOwners[cardID] = true
				} else {
					cardDependents[cardID] = true
				}
			}
			label := strings.TrimSpace(primitive.TypeName)
			if label == "" {
				label = strings.TrimSpace(params["TEXT"])
			}
			if label == "" && cardID != "" && cardID != "0" {
				if card, ok := cardIndex[cardID]; ok {
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
			summary.Supported = false
			summary.Warnings = appendStringOnce(summary.Warnings, "Неизвестный GROBJTYPE="+primitive.ObjectType)
		}
		cardID := strings.TrimSpace(primitive.CardID)
		if cardID != "" && cardID != "0" {
			cards[cardID] = struct{}{}
			if _, ok := cardIndex[cardID]; !ok {
				summary.Supported = false
				summary.Warnings = appendStringOnce(summary.Warnings, "CARDID "+cardID+" отсутствует в ISAOBJLIST")
			}
		}
	}
	for cardID := range cardDependents {
		if !cardOwners[cardID] {
			summary.Supported = false
			summary.Warnings = appendStringOnce(summary.Warnings, "CARDID "+cardID+" используется полями без блока-владельца")
		}
	}
	cardPrefixes := make(map[string]string)
	for cardID := range cards {
		card, exists := cardIndex[cardID]
		if !exists {
			continue
		}
		prefixKey := strings.ToUpper(strings.TrimSpace(card.EffectivePrefix()))
		if previous, duplicate := cardPrefixes[prefixKey]; duplicate && previous != cardID {
			summary.Supported = false
			summary.Warnings = appendStringOnce(summary.Warnings, "Карточки "+previous+" и "+cardID+" создают одинаковый Card.Info")
		} else {
			cardPrefixes[prefixKey] = cardID
		}
	}
	summary.CardCount = len(cards)
	preview.Width = max(preview.Width, 1)
	preview.Height = max(preview.Height, 1)
	summary.Preview = preview
	return summary
}

// ioCapabilities reports only the four physical binding profiles whose
// boundary semantics are evidenced by the native AI/AO/DI/DO exports.  It is
// deliberately structural: names shown to the user may change between
// libraries, while object IDs, free ports and link direction define whether a
// template can be connected without driving an already occupied endpoint.
func ioCapabilities(template *Template) []string {
	if template == nil {
		return []string{}
	}
	incoming := make(map[ioEndpoint]bool)
	outgoing := make(map[ioEndpoint]bool)
	for _, primitive := range template.Contents.Primitives {
		if !IsLinkType(primitive.ObjectType) {
			continue
		}
		params := ParseParams(primitive.Params)
		if value, ok := parseEndpointIdentity(params["FP"]); ok {
			outgoing[value] = true
		}
		if value, ok := parseEndpointIdentity(params["LP"]); ok {
			incoming[value] = true
		}
	}

	capabilities := make([]string, 0, 4)
	aiCandidates := 0
	aoCandidates := 0
	digitalBlocks := make([]string, 0)
	for _, primitive := range template.Contents.Primitives {
		id := strings.TrimSpace(primitive.ID)
		switch {
		case primitive.ObjectType == "37" && strings.TrimSpace(primitive.ISAObjectID) == "17480":
			if !incoming[ioEndpoint{id: id, port: "xin"}] && !incoming[ioEndpoint{id: id, port: "xs"}] {
				aiCandidates++
			}
		case primitive.ObjectType == "36" && strings.TrimSpace(primitive.ISAObjectID) == "791" &&
			strings.EqualFold(strings.TrimSpace(primitive.TypeName), "REAL_TO_DINT"):
			if !outgoing[ioEndpoint{id: id, port: "result"}] {
				aoCandidates++
			}
		case primitive.ObjectType == "31" && strings.TrimSpace(primitive.CardID) != "" && strings.TrimSpace(primitive.CardID) != "0":
			params := ParseParams(primitive.Params)
			if Int(params["CI"], 0) == 1 && Int(params["CO"], 0) == 1 {
				digitalBlocks = append(digitalBlocks, id)
			}
		}
	}
	if aiCandidates == 1 {
		capabilities = append(capabilities, "AI")
	}
	if aoCandidates == 1 {
		capabilities = append(capabilities, "AO")
	}
	diCandidates, doCandidates := 0, 0
	for _, id := range digitalBlocks {
		if !incoming[ioEndpoint{id: id, port: "0"}] {
			diCandidates++
		}
		if !outgoing[ioEndpoint{id: id, port: "0"}] {
			doCandidates++
		}
	}
	if diCandidates == 1 {
		capabilities = append(capabilities, "DI")
	}
	if doCandidates == 1 {
		capabilities = append(capabilities, "DO")
	}
	return capabilities
}

func parseEndpointIdentity(raw string) (ioEndpoint, bool) {
	parts := strings.SplitN(strings.TrimSpace(raw), "|", 4)
	if len(parts) != 4 {
		return ioEndpoint{}, false
	}
	return ioEndpoint{id: strings.TrimSpace(parts[0]), port: strings.ToLower(strings.TrimSpace(parts[2]))}, true
}

func appendStringOnce(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
