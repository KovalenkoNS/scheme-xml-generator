// XML-модели диагностики описывают HMI-страницы, изображения, примитивы и переходы между кадрами.
package hmi

import (
	"encoding/xml"
)

// This native profile is independent of program POUs. The names of external
// templates and symbols accompany their IDs so SCADA can resolve dependencies.
type plcDiagnosticDocument struct {
	XMLName    xml.Name               `xml:"BufScada"`
	Common     outputHMICommon        `xml:"Common"`
	Pages      []plcDiagnosticPage    `xml:"Pages>OnePage"`
	Groups     []plcDiagnosticGroup   `xml:"GRPAGESINFO>rec"`
	Colors     []outputHMIColor       `xml:"COLORSTYLES>rec"`
	Cards      []outputHMICard        `xml:"CARDSINFO>rec"`
	CardParams []outputHMIPageMS      `xml:"CARDPARAMSINFO>rec"`
	Pictures   []plcDiagnosticPicture `xml:"PICS>rec"`
	Symbols    []outputHMIPageMS      `xml:"PAGEMSINFO>rec"`
}

type plcDiagnosticGroup struct {
	ID       string `xml:"ID,attr"`
	FullName string `xml:"FullName,attr"`
}

type plcDiagnosticPicture struct {
	ID   string `xml:"ID,attr"`
	Name string `xml:"Name,attr"`
	Data string `xml:",chardata"`
}

type plcDiagnosticPage struct {
	IDAttribute string                 `xml:"ID,attr"`
	ID          string                 `xml:"ID"`
	Name        string                 `xml:"NAME"`
	TemplateID  string                 `xml:"SHABLONPAGEID"`
	ForMarka    string                 `xml:"FORMARKA"`
	ExData      string                 `xml:"EXDATA"`
	Background  string                 `xml:"FONCOLOR"`
	DParams     string                 `xml:"DPARAMS"`
	Height      string                 `xml:"HEIGHT"`
	GridSize    string                 `xml:"GRIDSIZE"`
	Width       string                 `xml:"WIDTH"`
	Number      string                 `xml:"NUM"`
	PrintWidth  string                 `xml:"PRINTWIDTH"`
	PrintHeight string                 `xml:"PRINTHEIGHT"`
	PrintPageA4 string                 `xml:"PRINTPAGEA4"`
	FrameNumber string                 `xml:"FRAMENUM"`
	Srez        string                 `xml:"SREZ"`
	Description string                 `xml:"DISC"`
	Layers      string                 `xml:"LAYERS"`
	ScriptCode  string                 `xml:"SCRIPTCODE"`
	PageLayers  []plcDiagnosticLayer   `xml:"PageLayers>OneLayer"`
	Children    *plcDiagnosticChildren `xml:"SUBPAGES,omitempty"`
}

type plcDiagnosticChildren struct {
	Pages []plcDiagnosticPage `xml:"OnePage"`
}

type plcDiagnosticLayer struct {
	Number     string                   `xml:"Num,attr"`
	Visible    string                   `xml:"Visible,attr"`
	Name       string                   `xml:"Name,attr"`
	Primitives []plcDiagnosticPrimitive `xml:"OnePrim"`
}

type plcDiagnosticPrimitive struct {
	T11ID       string                  `xml:"SourceT11ID,attr"`
	X           string                  `xml:"X,attr"`
	Y           string                  `xml:"Y,attr"`
	Width       string                  `xml:"WIDTH,attr"`
	Height      string                  `xml:"HEIGHT,attr"`
	ObjectType  string                  `xml:"OBJTYPE,attr"`
	GroupNumber string                  `xml:"GRNUM,attr"`
	DrawType    string                  `xml:"DRAWTYPE,attr"`
	PenParams   string                  `xml:"PenParams,attr"`
	PenColor    string                  `xml:"PenColor,attr"`
	BrushColor  string                  `xml:"BrushColor,attr"`
	GradColor   string                  `xml:"GradColor,attr"`
	ScriptName  string                  `xml:"ScriptName,attr"`
	PicID       string                  `xml:"PicId,attr,omitempty"`
	Params      string                  `xml:"PARAMS"`
	ObjectMSID  string                  `xml:"ObjMSID,omitempty"`
	CardID      string                  `xml:"CardID,omitempty"`
	Receptors   *plcDiagnosticReceptors `xml:"Receptors,omitempty"`
	Animators   *plcDiagnosticAnimators `xml:"Animators,omitempty"`
}

type plcDiagnosticReceptors struct {
	Items []plcDiagnosticReceptor `xml:"OneReceptor"`
}

type plcDiagnosticReceptor struct {
	ID      string               `xml:"FROMID,attr"`
	Type    string               `xml:"TYPEID,attr"`
	Int     string               `xml:"PARAM_INT,attr"`
	Float   string               `xml:"PARAM_FLOAT,attr"`
	Text    string               `xml:"PARAM_ST,attr"`
	Event   string               `xml:"EVPARAMS,attr"`
	DParams string               `xml:"DPARAMS,attr"`
	Rights  string               `xml:"USERRIGHTS,attr"`
	Key     string               `xml:"EVKEYPARAMS,attr"`
	Charts  *plcDiagnosticCharts `xml:"CHARTDATA,omitempty"`
}

type plcDiagnosticCharts struct {
	Items []plcDiagnosticChart `xml:"OneChart"`
}

type plcDiagnosticChart struct {
	IDAttribute string `xml:"ID,attr"`
	ID          string `xml:"ID"`
	ParentID    string `xml:"PID"`
	Mode        string `xml:"MODE"`
	ParamID     string `xml:"PARAMID"`
	Color       string `xml:"COLOR"`
	LSide       string `xml:"LSIDE"`
	NDParamID   string `xml:"NDPARAMID"`
	Style       string `xml:"CHARTSTYLE"`
	Width       string `xml:"WIDTH"`
	Stairs      string `xml:"STAIRS"`
	Marks       string `xml:"MARKS"`
	BitNum      string `xml:"BITNUM"`
	NDBitNum    string `xml:"NDBITNUM"`
	ParamMode   string `xml:"PARAMMODE"`
	TreePID     string `xml:"TREEPID"`
	TimeOffset  string `xml:"TIMEOFFSET"`
}

type plcDiagnosticAnimators struct {
	Items []plcDiagnosticAnimator `xml:"OneAnim"`
}

type plcDiagnosticAnimator struct {
	Attribute  string `xml:"ATRIBID,attr"`
	Param      string `xml:"PARAMID,attr"`
	Oper       string `xml:"OPER,attr"`
	Constant   string `xml:"CONSTVALUE,attr"`
	Action     string `xml:"ACTPARAM,attr"`
	Error      string `xml:"VARERROR,attr"`
	ScriptMode string `xml:"SCRIPTMODE,attr"`
	ScriptText string `xml:"SCRIPTTEXT,attr"`
	Array      string `xml:"ARRAYPARAMS,attr"`
	ParamMode  string `xml:"PARAMMODE,attr"`
}

type plcDiagnosticProfile struct {
	Pictures []plcDiagnosticPicture   `xml:"PICS>rec"`
	Colors   []outputHMIColor         `xml:"COLORSTYLES>rec"`
	Symbols  []outputHMIPageMS        `xml:"PAGEMSINFO>rec"`
	Front    []plcDiagnosticPrimitive `xml:"Front>OnePrim"`
	Back     []plcDiagnosticPrimitive `xml:"Back>OnePrim"`
}
