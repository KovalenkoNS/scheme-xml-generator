// Публичные JSON-контракты каталога типов, шаблонов и их предварительного просмотра.
package library

type Catalog struct {
	Libraries []LibrarySummary    `json:"libraries"`
	Templates []TemplateSummary   `json:"templates"`
	Types     []ObjectTypeSummary `json:"types"`
	Errors    []LoadError         `json:"errors"`
}

type LibrarySummary struct {
	File                   string   `json:"file"`
	Version                string   `json:"version"`
	TemplateCount          int      `json:"templateCount"`
	SupportedTemplateCount int      `json:"supportedTemplateCount"`
	TypeCount              int      `json:"typeCount"`
	Warnings               []string `json:"warnings"`
}

// ObjectTypeSummary exposes the library's own types even when an owner has
// no FBD template. A field's type is copied from ISAOBJ, never inferred.
type ObjectTypeSummary struct {
	LibraryFile   string             `json:"libraryFile"`
	ID            string             `json:"id"`
	Name          string             `json:"name"`
	TemplateCount int                `json:"templateCount"`
	Fields        []TypeFieldSummary `json:"fields"`
}

type TypeFieldSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	TypeName    string `json:"typeName"`
	LibraryName string `json:"libraryName"`
	Kind        string `json:"kind"`
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
