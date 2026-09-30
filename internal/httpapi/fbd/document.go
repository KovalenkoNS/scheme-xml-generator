// HTTP-оркестрация библиотечного FBD-документа; планирование и ID вынесены отдельно.
package fbd

import (
	"errors"
	"net/http"
	"scheme-xml-generator/internal/generator/allocation"
	fbdrequest "scheme-xml-generator/internal/generator/fbd/request"
	"scheme-xml-generator/internal/httpapi/transport"
)

// HandleDocumentGenerate координирует проверку, выполнение и выдачу запроса с массивом POU.
// Получает готовый JSON-контракт, передаёт план allocator и сохраняет XML только после успеха всех фаз.
func (s *Service) HandleDocumentGenerate(w http.ResponseWriter, request fbdrequest.Request) {
	plan, status, err := s.prepareDocument(request)
	if err != nil {
		transport.WriteError(w, status, err)
		return
	}
	result, err := s.generateDocument(plan)
	if err != nil {
		status := http.StatusBadRequest
		var persistence *allocation.AllocatorPersistenceError
		if errors.As(err, &persistence) {
			status = http.StatusInternalServerError
		}
		transport.WriteError(w, status, err)
		return
	}
	fallback := "multi_pou"
	if len(plan.POUs) == 1 && len(plan.POUs[0].Signals) == 1 {
		fallback = plan.POUs[0].Signals[0].Ref.Template.Name
	}
	s.Output.WriteGeneratedResult(w, plan.Request.FileName, fallback, result)
}
