// Контекст назначения программного XML, независимый от таблиц, HTTP и типа генератора.
package program

// ProgramContext содержит параметры Common/POU и выбранного драйвера для ST и библиотечного FBD.
// Значения предоставляет запрос или конкретный профиль; контракт не выбирает тип сигнала или область проекта.
type ProgramContext struct {
	Version            string `json:"version"`
	Project            string `json:"project"`
	ControllerTypeName string `json:"controllerTypeName"`
	PhysicalProfile    string `json:"physicalProfile,omitempty"`
	ControllerID       string `json:"controllerId"`
	ResourceID         string `json:"resourceId"`
	GroupID            string `json:"groupId"`
	POUNumber          string `json:"pouNumber"`
}
