// Генерация DO FBD исключительно из подключённой библиотеки.
package fbd

import (
	"errors"
	"fmt"
	"net/http"
	"scheme-xml-generator/internal/generator"

	"scheme-xml-generator/internal/httpapi/transport"
	"strings"
)

// HandleLibraryDO генерирует библиотечный DO-граф по выбранным физическим модулям.
// Проверяет шаблон и план до allocator, резервирует ID и передаёт сохранённый XML через output.Store.
func (s *Service) HandleLibraryDO(w http.ResponseWriter, r *http.Request) {
	var request generator.LibraryDORequest
	if err := transport.DecodeJSON(w, r, &request); err != nil {
		transport.WriteError(w, http.StatusBadRequest, err)
		return
	}
	ref, ok := s.Repository.Resolve(strings.TrimSpace(request.TemplateKey))
	if !ok {
		transport.WriteError(w, http.StatusNotFound, fmt.Errorf("DO: выберите шаблон подключённой библиотеки"))
		return
	}
	plan, err := s.Generator.PrepareLibraryDO(ref, request)
	if err != nil {
		transport.WriteError(w, http.StatusBadRequest, err)
		return
	}
	requirements := plan.Requirements()
	var result generator.Result
	_, err = s.Allocator.WithReservation(requirements.T11Count, requirements.CardCount, requirements.POUCount, generator.ReservationOptions{}, func(ids generator.IDRange) error {
		var generateErr error
		result, generateErr = s.Generator.GenerateLibraryDO(plan, ids)
		return generateErr
	})
	if err != nil {
		var persistenceErr *generator.AllocatorPersistenceError
		status := http.StatusBadRequest
		if errors.As(err, &persistenceErr) {
			status = http.StatusInternalServerError
		}
		transport.WriteError(w, status, err)
		return
	}
	s.Output.WriteGeneratedResult(w, request.FileName, "LIBRARY_DO_"+request.PLCName, result)
}
