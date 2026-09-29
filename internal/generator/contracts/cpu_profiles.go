// Контекст одного запроса переопределяет назначение библиотечной генерации без изменения config.json.
package contracts

// GenerationContext overrides the destination of one library FBD request.
// An omitted context preserves config.json, including its portable defaults.
// CPU selection never rewrites the application's configuration.
type GenerationContext struct {
	ControllerTypeName string  `json:"controllerTypeName"`
	Version            *string `json:"version,omitempty"`
	Project            *string `json:"project,omitempty"`
	ControllerID       *string `json:"controllerId,omitempty"`
	ResourceID         *string `json:"resourceId,omitempty"`
}
