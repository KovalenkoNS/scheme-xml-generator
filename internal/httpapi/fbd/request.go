// Декодирование и маршрутизация входного контракта HTTP.
package fbd

import (
	"errors"
	"fmt"
	"net/http"
	"scheme-xml-generator/internal/generator"
	"scheme-xml-generator/internal/httpapi/limits"
	"scheme-xml-generator/internal/library"

	"scheme-xml-generator/internal/httpapi/transport"
	"strings"
)

// HandleGenerate принимает запрос FBD по подключённой библиотеке из основного генератора.
// Различает одиночный и многопроцедурный запрос, проверяет совместимость, резервирует ID и сохраняет XML.
func (s *Service) HandleGenerate(w http.ResponseWriter, r *http.Request) {
	var request generator.Request
	if err := transport.DecodeJSON(w, r, &request); err != nil {
		transport.WriteError(w, http.StatusBadRequest, err)
		return
	}
	if request.POUs != nil {
		s.HandleDocumentGenerate(w, request)
		return
	}
	ref, ok := s.Repository.Resolve(request.TemplateKey)
	if !ok {
		transport.WriteError(w, http.StatusNotFound, fmt.Errorf("шаблон не найден; обновите список библиотек"))
		return
	}
	if supported, warnings := library.TemplateCompatibility(ref); !supported {
		transport.WriteError(w, http.StatusBadRequest, fmt.Errorf("шаблон %q несовместим с генератором: %s", ref.Template.Name, strings.Join(warnings, "; ")))
		return
	}
	t11Count, cardCount := generator.Requirements(ref)
	if t11Count > limits.MaxDocumentObjects || cardCount > limits.MaxDocumentCards {
		transport.WriteError(w, http.StatusBadRequest, fmt.Errorf("шаблон слишком велик: не более %d графических объектов и %d карточек", limits.MaxDocumentObjects, limits.MaxDocumentCards))
		return
	}
	var result generator.Result
	_, err := s.Allocator.WithReservation(t11Count, cardCount, 1, generator.ReservationOptions{
		T11Start: request.T11Start, CardStart: request.CardStart, POUIDs: []*int64{request.POUID},
	}, func(ids generator.IDRange) error {
		var generateErr error
		result, generateErr = s.Generator.Generate(ref, request, ids)
		return generateErr
	})
	if err != nil {
		var persistenceErr *generator.AllocatorPersistenceError
		if errors.As(err, &persistenceErr) {
			transport.WriteError(w, http.StatusInternalServerError, err)
		} else {
			transport.WriteError(w, http.StatusBadRequest, err)
		}
		return
	}
	s.Output.WriteGeneratedResult(w, request.FileName, ref.Template.Name, result)
}
