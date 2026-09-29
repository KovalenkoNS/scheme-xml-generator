// Единая транзакция ID для заранее проверенного FBD-документа.
package fbd

import (
	"scheme-xml-generator/internal/generator"
)

// generateDocument запускает генератор внутри reservation для готового плана POU.
// Возвращает XML или ошибку и позволяет allocator зафиксировать ID только после успешной генерации.
func (s *Service) generateDocument(plan preparedDocument) (generator.Result, error) {
	var result generator.Result
	_, err := s.Allocator.WithReservation(plan.Requirements.T11Count, plan.Requirements.CardCount, plan.Requirements.POUCount, generator.ReservationOptions{
		T11Start: plan.Request.T11Start, CardStart: plan.Request.CardStart, POUIDs: plan.POUIDs,
	}, func(ids generator.IDRange) error {
		var generateErr error
		result, generateErr = s.Generator.GenerateDocument(plan.Request, plan.POUs, ids)
		return generateErr
	})
	return result, err
}
