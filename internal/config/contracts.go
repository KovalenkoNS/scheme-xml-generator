// Контракты данных компонента без файлового ввода-вывода.
package config

type Config struct {
	ListenAddress string         `json:"listenAddress"`
	Port          int            `json:"port"`
	AutoOpen      bool           `json:"autoOpenBrowser"`
	Common        CommonDefaults `json:"common"`
	Page          PageDefaults   `json:"page"`
	IDs           IDDefaults     `json:"ids"`
}

type CommonDefaults struct {
	Version        string `json:"version"`
	Project        string `json:"project"`
	ControllerType string `json:"controllerTypeName"`
	ControllerID   string `json:"controllerId"`
	ResourceID     string `json:"resourceId"`
}

type PageDefaults struct {
	DParams      string `json:"dparams"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Background   string `json:"backgroundColor"`
	GroupID      string `json:"groupId"`
	POUNumber    string `json:"pouNumber"`
	MarginRight  int    `json:"marginRight"`
	MarginBottom int    `json:"marginBottom"`
}

type IDDefaults struct {
	NextT11  int64 `json:"nextT11"`
	NextCard int64 `json:"nextCard"`
	NextPOU  int64 `json:"nextPou"`
	NextPage int64 `json:"nextPage"`
}
