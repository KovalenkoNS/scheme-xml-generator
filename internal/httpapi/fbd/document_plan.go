// Проверка единого плана библиотечных POU до выделения ID.
package fbd

import (
	"fmt"
	"net/http"
	"scheme-xml-generator/internal/generator"
	"scheme-xml-generator/internal/httpapi/limits"
	"strings"
)

type preparedDocument struct {
	Request      generator.Request
	POUs         []generator.ResolvedPOU
	Requirements generator.DocumentRequirements
	POUIDs       []*int64
}

// prepareDocument отклоняет прежнее аппаратное дополнение, разрешает библиотечные ссылки и считает FBD-план.
// Возвращает нормализованный запрос/требования ID либо ошибку со статусом; allocator и output не изменяет.
func (s *Service) prepareDocument(request generator.Request) (preparedDocument, int, error) {
	for _, pou := range request.POUs {
		if pou.IO != nil {
			return preparedDocument{}, http.StatusGone, fmt.Errorf("формат io.modules отключён. FBD создаётся только из подключённой библиотеки. Выберите шаблон на основной странице")
		}
	}
	normalized, err := generator.NormalizePOURequests(request.POUs)
	if err != nil {
		return preparedDocument{}, http.StatusBadRequest, err
	}
	request.POUs = normalized
	if hasLegacyGenerateFields(request) {
		return preparedDocument{}, http.StatusBadRequest, fmt.Errorf("при использовании поля pous настройки POU и сигналов должны находиться внутри соответствующей POU")
	}
	if len(request.POUs) > limits.MaxDocumentPOUs {
		return preparedDocument{}, http.StatusBadRequest, fmt.Errorf("один файл может содержать не более %d POU", limits.MaxDocumentPOUs)
	}

	keys, err := collectTemplateKeys(request)
	if err != nil {
		return preparedDocument{}, http.StatusBadRequest, err
	}
	references, missing := s.Repository.ResolveMany(keys)
	if len(missing) > 0 {
		return preparedDocument{}, http.StatusNotFound, fmt.Errorf("шаблоны не найдены; обновите список библиотек: %s", strings.Join(missing, ", "))
	}
	resolved, err := resolvePOUSignals(request, references)
	if err != nil {
		return preparedDocument{}, http.StatusBadRequest, err
	}
	requirements, err := generator.RequirementsForDocument(resolved)
	if err != nil {
		return preparedDocument{}, http.StatusBadRequest, err
	}
	if requirements.T11Count > limits.MaxDocumentObjects || requirements.CardCount > limits.MaxDocumentCards {
		return preparedDocument{}, http.StatusBadRequest, fmt.Errorf("документ слишком велик: не более %d графических объектов и %d карточек", limits.MaxDocumentObjects, limits.MaxDocumentCards)
	}
	pouIDs := make([]*int64, len(request.POUs))
	for index := range request.POUs {
		pouIDs[index] = request.POUs[index].POUID
	}

	return preparedDocument{Request: request, POUs: resolved, Requirements: requirements, POUIDs: pouIDs}, 0, nil
}

// hasLegacyGenerateFields обнаруживает смешение прежних одиночных полей и массива pous.
// Возвращает наличие конфликтующих настроек, чтобы FBD-обработчик отклонил неоднозначный запрос до генерации.
func hasLegacyGenerateFields(request generator.Request) bool {
	return strings.TrimSpace(request.TemplateKey) != "" ||
		strings.TrimSpace(request.ObjectName) != "" ||
		strings.TrimSpace(request.POUName) != "" ||
		strings.TrimSpace(request.NameMode) != "" ||
		strings.TrimSpace(request.Description) != "" ||
		strings.TrimSpace(request.ClusterPath) != "" ||
		request.OffsetX != nil || request.OffsetY != nil || request.POUID != nil || request.POUGroupID != nil || request.POUNumber != nil
}
