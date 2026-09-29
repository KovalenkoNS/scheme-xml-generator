// Контракты данных компонента без файлового ввода-вывода.
package techobjects

type Summary struct {
	ModuleCount  int `json:"moduleCount"`
	ReserveCount int `json:"reserveCount"`
	ObjectCount  int `json:"objectCount"`
}

type TypeCounts struct {
	AI int `json:"ai"`
	AO int `json:"ao"`
	DI int `json:"di"`
	DO int `json:"do"`
}

type ControllerPreview struct {
	Key              string     `json:"key"`
	Name             string     `json:"name"`
	SourceController string     `json:"sourceFcs"` // Совместимое имя поля preview; внутри используется модель ПЛК.
	Cabinet          string     `json:"cabinet"`
	Types            TypeCounts `json:"types"`
	Summary
}

type Object struct {
	Tag, Type, Name, Sign, Template string
	Texting                         [3]string
}

type Plan struct {
	ControllerName string
	Resource       int
	Objects        []Object
	Summary        Summary
}
