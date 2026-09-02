package generator

import "encoding/xml"

type outputDocument struct {
	XMLName    xml.Name          `xml:"BufScadaPOUS"`
	Common     outputCommon      `xml:"Common"`
	POUS       outputPOUS        `xml:"POUS"`
	FontStyles *outputFontStyles `xml:"FONTSTYLES,omitempty"`
	ISAObjects outputISAObjects  `xml:"ISAOBJSINFO"`
	ISACards   outputISACards    `xml:"ISACARDSINFO"`
}

type outputCommon struct {
	Version        string `xml:"VER,attr"`
	Project        string `xml:"Project,attr"`
	IsCut          string `xml:"isCut,attr"`
	IsFFB          string `xml:"isFFB,attr"`
	ControllerType string `xml:"ControllerTypeName,attr"`
	ControllerID   string `xml:"ControllerID,attr"`
	ResourceID     string `xml:"ResuorceID,attr"`
}

type outputPOUS struct {
	Items []outputPOU `xml:"OnePOU"`
}

type outputPOU struct {
	ID          string          `xml:"ID,attr"`
	Name        string          `xml:"NAME,attr"`
	IsFBD       string          `xml:"isFBD,attr"`
	GroupID     string          `xml:"GroupID,attr"`
	Enabled     string          `xml:"Enabled,attr"`
	Number      string          `xml:"POUNum,attr"`
	Description string          `xml:"Disc,attr"`
	Params      outputPOUParams `xml:"PARAMS"`
	ISAGraf     outputISAGraf   `xml:"ISAGraf"`
	Graphics    outputGraphics  `xml:"GrObj"`
}

type outputPOUParams struct {
	DParams      string `xml:"DPARAMS,attr"`
	Height       string `xml:"HEIGHT,attr"`
	Width        string `xml:"WIDTH,attr"`
	TemplatePage string `xml:"SHABLONPAGEID,attr"`
	Background   string `xml:"FONCOLOR,attr"`
	PrintWidth   string `xml:"PRINTWIDTH,attr"`
	PrintHeight  string `xml:"PRINTHEIGHT,attr"`
	PrintPageA4  string `xml:"PRINTPAGEA4,attr"`
}

type outputISAGraf struct {
	Blocks outputBlocks `xml:"Blocks"`
	Gotos  outputGotos  `xml:"Gotos"`
	Links  outputLinks  `xml:"Links"`
}

type outputGotos struct{}

type outputBlocks struct {
	Items []outputBlock `xml:"Block"`
}

type outputBlock struct {
	ObjectType string            `xml:"GROBJTYPE,attr"`
	Info       string            `xml:"Info,attr"`
	T11ID      string            `xml:"T11ID,attr"`
	Graphics   outputBlockBounds `xml:"Graphics"`
	Params     outputBlockParams `xml:"Params"`
}

type outputBlockBounds struct {
	X      string `xml:"X,attr"`
	Y      string `xml:"Y,attr"`
	Width  string `xml:"WIDTH,attr"`
	Height string `xml:"HEIGHT,attr"`
}

type outputBlockParams struct {
	Text        string  `xml:"T11Text,attr"`
	CI          string  `xml:"CI,attr"`
	CO          string  `xml:"CO,attr"`
	ViewMode    string  `xml:"VMODE,attr"`
	Commented   string  `xml:"Commented,attr"`
	ISAObjectID string  `xml:"IsaObjId,attr"`
	CardID      string  `xml:"cardId,attr"`
	Initial     *string `xml:"IV,omitempty"`
}

type outputLinks struct {
	Items []outputLink `xml:"Link"`
}

type outputLink struct {
	Negative   string          `xml:"Negative,attr"`
	AsPointer  string          `xml:"asPointer,attr"`
	UserEdit   string          `xml:"UserEdit,attr"`
	Color      string          `xml:"Color,attr"`
	GetBitNum  string          `xml:"GetBitNum,attr"`
	ConvertTo  string          `xml:"ConvertTo,attr"`
	PointList  outputPointList `xml:"PointList"`
	FirstPoint outputEndpoint  `xml:"FirstPoint"`
	LastPoint  outputEndpoint  `xml:"LastPoint"`
}

type outputPointList struct {
	Points string `xml:"PL,attr"`
}

type outputEndpoint struct {
	Value string `xml:"FP,attr,omitempty"`
	Last  string `xml:"LP,attr,omitempty"`
}

type outputGraphics struct {
	Items []outputPrimitive `xml:"OnePrim"`
}

type outputPrimitive struct {
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

type outputFontStyles struct {
	Items []outputFontStyle `xml:"rec"`
}

type outputFontStyle struct {
	ID        string `xml:"ID,attr"`
	Name      string `xml:"Name,attr"`
	FontName  string `xml:"FontName,attr"`
	FontColor string `xml:"FontColor,attr"`
	FontSize  string `xml:"FontSize,attr"`
	FontParam string `xml:"FontParam,attr"`
}

type outputISAObjects struct {
	Items []outputISAObject `xml:"rec"`
}

type outputISAObject struct {
	ID          string `xml:"ID,attr"`
	Info        string `xml:"Info,attr"`
	LibraryName string `xml:"LibName,attr"`
}

type outputISACards struct {
	Items []outputISACard `xml:"rec"`
}

type outputISACard struct {
	ID          string `xml:"ID,attr"`
	Info        string `xml:"Info,attr"`
	IsRetain    string `xml:"IsRetain,attr"`
	Name        string `xml:"Name,attr"`
	Size        string `xml:"SSize,attr"`
	ClusterPath string `xml:"KLPath,attr"`
}
