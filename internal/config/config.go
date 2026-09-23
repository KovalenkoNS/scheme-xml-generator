package config

import (
	"encoding/json"
	"fmt"
	"os"
)

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

func Default() Config {
	return Config{
		ListenAddress: "127.0.0.1",
		Port:          3210,
		AutoOpen:      true,
		Common: CommonDefaults{
			Version: "29", Project: "", ControllerType: "", ControllerID: "0", ResourceID: "0",
		},
		Page: PageDefaults{
			DParams: "3", Width: 2000, Height: 2000, Background: "16777215", GroupID: "0", POUNumber: "0", MarginRight: 200, MarginBottom: 200,
		},
		IDs: IDDefaults{NextT11: 3000000, NextCard: 900000, NextPOU: 100000, NextPage: 1000000},
	}
}

func Load(path string) (Config, error) {
	result := Default()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("прочитать config.json: %w", err)
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, fmt.Errorf("разобрать config.json: %w", err)
	}
	if result.ListenAddress == "" {
		result.ListenAddress = "127.0.0.1"
	}
	if result.Port < 1 || result.Port > 65535 {
		return result, fmt.Errorf("port должен быть в диапазоне 1..65535")
	}
	return result, nil
}
