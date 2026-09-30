// HTTP-оркестрация ST-перекладки; чтение, планирование и ID выполняются отдельными фазами.
package moduleassignment

import (
	"errors"
	"net/http"
	"scheme-xml-generator/internal/generator/allocation"
	"scheme-xml-generator/internal/httpapi/transport"
)

// HandleModuleAssignmentGenerate принимает ST-запрос канонического API mappings и исторического URL alias.
// Связывает чтение/план/генерацию/выдачу; запрещённый встроенный FBD отклоняется ещё на фазе чтения.
func (s *Service) HandleModuleAssignmentGenerate(w http.ResponseWriter, r *http.Request) {
	input, status, err := readAssignmentInput(w, r)
	if err != nil {
		transport.WriteError(w, status, err)
		return
	}
	plan, err := prepareAssignmentPlan(input)
	if err != nil {
		transport.WriteError(w, http.StatusBadRequest, err)
		return
	}
	items, err := s.generateAssignments(plan)
	if err != nil {
		status := http.StatusBadRequest
		var persistence *allocation.AllocatorPersistenceError
		if errors.As(err, &persistence) {
			status = http.StatusInternalServerError
		}
		transport.WriteError(w, status, err)
		return
	}
	s.Output.WriteControllerBatch(w, input.Request.Kind, items, plan.Warnings)
}
