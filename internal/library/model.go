// XML-модель подключённой библиотеки и контракты загруженных типов/шаблонов.
package library

import "encoding/xml"

type Document struct {
	XMLName        xml.Name    `xml:"root"`
	Version        Version     `xml:"SCADATA_VER"`
	Sections       []Section   `xml:"SECTION"`
	RootFontStyles []FontStyle `xml:"FONTSTYLES>rec"`
}

type Version struct {
	Value string `xml:"VER,attr"`
}

type Section struct {
	Number string `xml:"Num,attr"`
	Text   string `xml:"Text,attr"`
	Other  Other  `xml:"OTHER"`
}

type Other struct {
	ObjectTypes []ObjectType `xml:"OBJTYPE"`
}

type ObjectType struct {
	ID         string        `xml:"ID,attr"`
	Name       string        `xml:"Name,attr"`
	Templates  Templates     `xml:"TEMPLATES"`
	ISAObjects ISAObjectList `xml:"ISAOBJLIST"`
	Child      ObjectChild   `xml:"CHILD"`
}

type ObjectChild struct {
	ObjectTypes []ObjectType `xml:"OBJTYPE"`
}

type Templates struct {
	Items []Template `xml:"TEMPLATE"`
}

type Template struct {
	ID          string   `xml:"ID,attr"`
	Name        string   `xml:"Name,attr"`
	Description string   `xml:"DISC"`
	DParams     string   `xml:"DPARAMS"`
	Height      string   `xml:"HEIGHT"`
	Width       string   `xml:"WIDTH"`
	Background  string   `xml:"FONCOLOR"`
	Contents    Contents `xml:"CONTENTS"`
}

type Contents struct {
	Primitives []Primitive `xml:"grprim"`
}

type Primitive struct {
	ID                string  `xml:"ID"`
	ObjectType        string  `xml:"GROBJTYPE"`
	PageID            string  `xml:"PAGEID"`
	X                 string  `xml:"X"`
	Y                 string  `xml:"Y"`
	Width             string  `xml:"WIDTH"`
	Height            string  `xml:"HEIGHT"`
	MSort             string  `xml:"MSORT"`
	Params            string  `xml:"PARAMS"`
	DrawType          string  `xml:"DRAWTYPE"`
	PenColor          string  `xml:"PENCOLOR"`
	BrushColor        string  `xml:"BRUSHCOLOR"`
	PenParams         string  `xml:"PENPARAMS"`
	CardID            string  `xml:"CARDID"`
	ISAObjectID       string  `xml:"OBJMSID"`
	GraphicNumber     string  `xml:"GRNUM"`
	GradientColor     string  `xml:"GRADCOLOR"`
	Name              string  `xml:"NAME"`
	GroupID           string  `xml:"GROUPID"`
	FieldsID          string  `xml:"FIELDSID"`
	LayerNumber       string  `xml:"LAYERNUM"`
	TemplateGraphicID string  `xml:"TEMPLATEGROBJID"`
	ExtraParams       string  `xml:"EXPARAMS"`
	TypeName          string  `xml:"TYPENAME"`
	LibraryName       string  `xml:"LIBNAME"`
	InitialValue      *string `xml:"IV"`
}

type ISAObjectList struct {
	Items []ISAObject `xml:"ISAOBJ"`
}

type ISAObject struct {
	ID           string  `xml:"ID,attr"`
	PrefixAttr   string  `xml:"Prefix,attr"`
	Prefix       string  `xml:"PREFIX"`
	Description  string  `xml:"DISC"`
	Local        string  `xml:"LOCAL"`
	InitialValue *string `xml:"INITIALVALUE"`
	Size         string  `xml:"SSIZE"`
	TypeName     string  `xml:"ISATNAME"`
	LibraryName  string  `xml:"LIBNAME"`
	Kind         string  `xml:"KINDOBJ"`
}

// EffectivePrefix читает имя поля объекта из двух поддерживаемых представлений ISAOBJ.
// Возвращает атрибут PREFIX, если задан, иначе одноимённый вложенный элемент библиотеки.
func (o ISAObject) EffectivePrefix() string {
	if o.PrefixAttr != "" {
		return o.PrefixAttr
	}
	return o.Prefix
}

type FontStyle struct {
	ID        string `xml:"ID,attr"`
	Name      string `xml:"Name,attr"`
	FontName  string `xml:"FontName,attr"`
	FontColor string `xml:"FontColor,attr"`
	FontSize  string `xml:"FontSize,attr"`
	FontParam string `xml:"FontParam,attr"`
}

type Signature struct {
	CI int
	CO int
}

type LoadedLibrary struct {
	FileName       string
	Path           string
	Version        string
	Warnings       []string
	Document       *Document
	FontStyles     map[string]FontStyle
	TypeSignatures map[string]Signature
}

type TemplateRef struct {
	Key      string
	Library  *LoadedLibrary
	Owner    *ObjectType
	Template *Template
}

// SignatureKey создаёт составной ключ сигнатуры функции из ID объекта и имени типа библиотеки.
// Возвращает строку с нулевым разделителем, исключающим неоднозначную склейку полей.
func SignatureKey(objectID, typeName string) string {
	return objectID + "\x00" + typeName
}

// IsLinkType классифицирует примитив библиотеки перед анализом связей и генерацией.
// Возвращает true только для подтверждённого GROBJTYPE 20.
func IsLinkType(objectType string) bool {
	return objectType == "20"
}

// IsGraphicType отделяет графические примитивы от логических блоков библиотечного шаблона.
// Принимает код GROBJTYPE и возвращает поддержку подтверждённых графических типов 1, 2 или 7.
func IsGraphicType(objectType string) bool {
	switch objectType {
	case "1", "2", "7":
		return true
	default:
		return false
	}
}

// IsSupportedBlockType проверяет код логического блока при чтении и генерации библиотечного FBD.
// Возвращает поддержку подтверждённых типов 31/34/35/36/37, не выводя её из имени объекта.
func IsSupportedBlockType(objectType string) bool {
	switch objectType {
	case "31", "34", "35", "36", "37":
		return true
	default:
		return false
	}
}
