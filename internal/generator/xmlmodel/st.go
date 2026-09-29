// ST transport structures preserve BufScadaPOUS without graphical sections.
package xmlmodel

import "encoding/xml"

type OutputSTDocument struct {
	XMLName xml.Name     `xml:"BufScadaPOUS"`
	Common  OutputCommon `xml:"Common"`
	POUS    OutputSTPOUS `xml:"POUS"`
}

type OutputSTPOUS struct {
	Items []OutputSTPOU `xml:"OnePOU"`
}

type OutputSTPOU struct {
	ID          string `xml:"ID,attr"`
	Name        string `xml:"NAME,attr"`
	IsFBD       string `xml:"isFBD,attr"`
	GroupID     string `xml:"GroupID,attr"`
	Enabled     string `xml:"Enabled,attr"`
	Number      string `xml:"POUNum,attr"`
	Description string `xml:"Disc,attr"`
	Code        string `xml:"STCODE"`
}
