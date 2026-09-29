// Начальные настройки запуска, XML и идентификаторов без чтения окружения.
package config

// Default предоставляет исходные настройки локального запуска, страницы XML и диапазонов ID.
// Возвращает новую Config без чтения файлов; неизвестные проект и тип ПЛК остаются пустыми.
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
