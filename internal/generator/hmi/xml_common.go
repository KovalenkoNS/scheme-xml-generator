// Общие XML-записи BufScada для HMI-профилей: назначение, цвета и привязки карточек.
package hmi

type outputHMICommon struct {
	Version string `xml:"VER,attr"`
	Project string `xml:"Project,attr"`
}

type outputHMIColor struct {
	ID    string `xml:"ID,attr"`
	Name  string `xml:"Name,attr"`
	Color string `xml:"Color,attr"`
}

type outputHMICard struct {
	ID   string `xml:"ID,attr"`
	Info string `xml:"CardInfo,attr"`
}

type outputHMIPageMS struct {
	ID   string `xml:"ID,attr"`
	Info string `xml:"Info,attr"`
}
