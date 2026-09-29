// Транспортные структуры FBD описывают документ SCADA, графические примитивы, карточки и таблицы типов.
package xmlmodel

import (
	"encoding/xml"
)

type OutputDocument struct {
	XMLName    xml.Name          `xml:"BufScadaPOUS"`
	Common     OutputCommon      `xml:"Common"`
	POUS       OutputPOUS        `xml:"POUS"`
	FontStyles *OutputFontStyles `xml:"FONTSTYLES,omitempty"`
	ISAObjects OutputISAObjects  `xml:"ISAOBJSINFO"`
	ISACards   OutputISACards    `xml:"ISACARDSINFO"`
}

type OutputCommon struct {
	Version        string `xml:"VER,attr"`
	Project        string `xml:"Project,attr"`
	IsCut          string `xml:"isCut,attr"`
	IsFFB          string `xml:"isFFB,attr"`
	ControllerType string `xml:"ControllerTypeName,attr"`
	ControllerID   string `xml:"ControllerID,attr"`
	ResourceID     string `xml:"ResuorceID,attr"`
}

type OutputPOUS struct {
	Items []OutputPOU `xml:"OnePOU"`
}

type OutputPOU struct {
	ID          string          `xml:"ID,attr"`
	Name        string          `xml:"NAME,attr"`
	IsFBD       string          `xml:"isFBD,attr"`
	GroupID     string          `xml:"GroupID,attr"`
	Enabled     string          `xml:"Enabled,attr"`
	Number      string          `xml:"POUNum,attr"`
	Description string          `xml:"Disc,attr"`
	Params      OutputPOUParams `xml:"PARAMS"`
	ISAGraf     OutputISAGraf   `xml:"ISAGraf"`
	Graphics    OutputGraphics  `xml:"GrObj"`
}

type OutputPOUParams struct {
	DParams      string `xml:"DPARAMS,attr"`
	Height       string `xml:"HEIGHT,attr"`
	Width        string `xml:"WIDTH,attr"`
	TemplatePage string `xml:"SHABLONPAGEID,attr"`
	Background   string `xml:"FONCOLOR,attr"`
	PrintWidth   string `xml:"PRINTWIDTH,attr"`
	PrintHeight  string `xml:"PRINTHEIGHT,attr"`
	PrintPageA4  string `xml:"PRINTPAGEA4,attr"`
}

type OutputISAGraf struct {
	Blocks OutputBlocks `xml:"Blocks"`
	Gotos  OutputGotos  `xml:"Gotos"`
	Links  OutputLinks  `xml:"Links"`
}

type OutputGotos struct{}

type OutputBlocks struct {
	Items []OutputBlock `xml:"Block"`
}

type OutputBlock struct {
	ObjectType string            `xml:"GROBJTYPE,attr"`
	Info       string            `xml:"Info,attr"`
	T11ID      string            `xml:"T11ID,attr"`
	Graphics   OutputBlockBounds `xml:"Graphics"`
	Params     OutputBlockParams `xml:"Params"`
}

type OutputBlockBounds struct {
	X      string `xml:"X,attr"`
	Y      string `xml:"Y,attr"`
	Width  string `xml:"WIDTH,attr"`
	Height string `xml:"HEIGHT,attr"`
}

type OutputBlockParams struct {
	Text        string  `xml:"T11Text,attr"`
	CI          string  `xml:"CI,attr"`
	CO          string  `xml:"CO,attr"`
	ViewMode    string  `xml:"VMODE,attr"`
	Commented   string  `xml:"Commented,attr"`
	ISAObjectID string  `xml:"IsaObjId,attr"`
	CardID      string  `xml:"cardId,attr"`
	Initial     *string `xml:"IV,omitempty"`
}

type OutputLinks struct {
	Items []OutputLink `xml:"Link"`
}

type OutputLink struct {
	Negative   string          `xml:"Negative,attr"`
	AsPointer  string          `xml:"asPointer,attr"`
	UserEdit   string          `xml:"UserEdit,attr"`
	Color      string          `xml:"Color,attr"`
	GetBitNum  string          `xml:"GetBitNum,attr"`
	ConvertTo  string          `xml:"ConvertTo,attr"`
	PointList  OutputPointList `xml:"PointList"`
	FirstPoint OutputEndpoint  `xml:"FirstPoint"`
	LastPoint  OutputEndpoint  `xml:"LastPoint"`
}

type OutputPointList struct {
	Points string `xml:"PL,attr"`
}

type OutputEndpoint struct {
	Value string `xml:"FP,attr,omitempty"`
	Last  string `xml:"LP,attr,omitempty"`
}

type OutputGraphics struct {
	Items []OutputPrimitive `xml:"OnePrim"`
}

type OutputPrimitive struct {
	SourceT11ID string `xml:"SourceT11ID,attr"`
	X           string `xml:"X,attr"`
	Y           string `xml:"Y,attr"`
	Width       string `xml:"WIDTH,attr"`
	Height      string `xml:"HEIGHT,attr"`
	ObjectType  string `xml:"OBJTYPE,attr"`
	GraphicNo   string `xml:"GRNUM,attr"`
	DrawType    string `xml:"DRAWTYPE,attr"`
	PenParams   string `xml:"PenParams,attr"`
	PenColor    string `xml:"PenColor,attr"`
	BrushColor  string `xml:"BrushColor,attr"`
	Gradient    string `xml:"GradColor,attr"`
	ScriptName  string `xml:"ScriptName,attr"`
	Params      string `xml:"PARAMS"`
}

type OutputFontStyles struct {
	Items []OutputFontStyle `xml:"rec"`
}

type OutputFontStyle struct {
	ID        string `xml:"ID,attr"`
	Name      string `xml:"Name,attr"`
	FontName  string `xml:"FontName,attr"`
	FontColor string `xml:"FontColor,attr"`
	FontSize  string `xml:"FontSize,attr"`
	FontParam string `xml:"FontParam,attr"`
}

type OutputISAObjects struct {
	Items []OutputISAObject `xml:"rec"`
}

type OutputISAObject struct {
	ID          string `xml:"ID,attr"`
	Info        string `xml:"Info,attr"`
	LibraryName string `xml:"LibName,attr"`
}

type OutputISACards struct {
	Items []OutputISACard `xml:"rec"`
}

type OutputISACard struct {
	ID          string `xml:"ID,attr"`
	Info        string `xml:"Info,attr"`
	IsRetain    string `xml:"IsRetain,attr"`
	Name        string `xml:"Name,attr"`
	Size        string `xml:"SSize,attr"`
	ClusterPath string `xml:"KLPath,attr"`
}
