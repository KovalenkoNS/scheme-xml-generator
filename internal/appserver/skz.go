package appserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"scheme-xml-generator/internal/generator"
	"scheme-xml-generator/internal/iomap"
	"scheme-xml-generator/internal/skzmap"
)

func (s *Server) handleSKZProfile(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"context":     generator.DefaultSKZContext(),
		"contexts":    map[string]generator.AOMappingContext{"AI": skzContextForMode("AI", "st"), "DO": skzContextForMode("DO", "st")},
		"fbdContexts": map[string]generator.AOMappingContext{"AI": skzContextForMode("AI", "fbd"), "DO": skzContextForMode("DO", "fbd")},
		"description": "СКЗ PLC 850: перекладки AI/DO из XLSX. Количество модулей задаётся для каждой группы; дополнительные модули заполняются резервными каналами. В AI FBD каждый модуль занимает отдельную POU (AI_A1_00, AI_A1_01 и далее), до 16 сигналов с привязкой к тегам карты. DO FBD подключает сигналы ко входам D32V. Один XML на ПЛК. Числовые ID модулей для ST задаются по конфигурации целевого ПЛК.",
	})
}

func skzContextForMode(kind, mode string) generator.AOMappingContext {
	ctx := generator.DefaultSKZContext()
	if kind == "DO" {
		ctx.GroupID, ctx.POUNumber = "19913", "47"
	} else if mode == "fbd" {
		ctx.POUNumber = "8" // SOGO_AI_fbd.xml; AI ST starts at 4.
	}
	return ctx
}

func readSKZUpload(w http.ResponseWriter, r *http.Request) ([]byte, string, string, error) {
	return readMappingUpload(w, r, iomap.MaxWorkbookBytes, "XLSX", "16", "config")
}

func (s *Server) handleSKZPreview(w http.ResponseWriter, r *http.Request) {
	data, _, _, err := readSKZUpload(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	plan, err := skzmap.Parse(data)
	if err == nil {
		err = checkSKZSize(plan)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

func (s *Server) handleSKZGenerate(w http.ResponseWriter, r *http.Request) {
	data, fileName, rawContext, err := readSKZUpload(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	request, err := decodeSKZRequest(r.MultipartForm.Value["config"])
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	source, err := skzmap.Parse(data)
	if err == nil {
		err = checkSKZSize(source)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx, err := decodeMappingContextWithDefault(rawContext, skzContextForMode(source.Groups[0].Kind, request.Kind))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	plans, err := generator.PrepareSKZPlans(source, request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	requirements := make([]generator.DocumentRequirements, len(plans))
	var total generator.DocumentRequirements
	for index, plan := range plans {
		requirements[index], err = generator.RequirementsForSKZ(plan)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		total.T11Count += requirements[index].T11Count
		total.CardCount += requirements[index].CardCount
		total.POUCount += requirements[index].POUCount
		total.SignalCount += requirements[index].SignalCount
	}
	if total.SignalCount > maxDocumentSignals {
		writeError(w, http.StatusBadRequest, fmt.Errorf("перекладка СКЗ с дополнительными модулями превышает предел %d каналов", maxDocumentSignals))
		return
	}
	if total.POUCount > maxDocumentPOUs {
		writeError(w, http.StatusBadRequest, fmt.Errorf("перекладка СКЗ превышает предел %d POU с учётом отдельных POU модулей AI FBD", maxDocumentPOUs))
		return
	}
	if total.T11Count > maxDocumentObjects || total.CardCount > maxDocumentCards {
		writeError(w, http.StatusBadRequest, fmt.Errorf("перекладка СКЗ превышает допустимое число объектов или POU"))
		return
	}
	items := make([]aoBatchItem, len(plans))
	_, err = s.allocator.WithReservation(total.T11Count, total.CardCount, total.POUCount, generator.ReservationOptions{}, func(ids generator.IDRange) error {
		for index, plan := range plans {
			result, generateErr := s.generator.GenerateSKZ(plan, ctx, ids)
			if generateErr != nil {
				return generateErr
			}
			base := strings.TrimSuffix(safeOutputName(fileName, "SKZ", "mapping"), ".xml")
			controller := strings.TrimSuffix(safeOutputName(plan.SCS, "SCS", "mapping"), ".xml")
			items[index] = aoBatchItem{FCS: plan.SCS, FileName: base + "_" + strings.ToUpper(request.Kind) + "_" + controller + ".xml", Result: result,
				AssignmentCount: plan.AssignmentCount, RepeatedAssignmentCount: plan.RepeatedAssignmentCount}
			ids.T11Start += int64(requirements[index].T11Count)
			ids.CardStart += int64(requirements[index].CardCount)
			ids.POUID += int64(requirements[index].POUCount)
		}
		return nil
	})
	if err != nil {
		status := http.StatusBadRequest
		var persistenceErr *generator.AllocatorPersistenceError
		if errors.As(err, &persistenceErr) {
			status = http.StatusInternalServerError
		}
		writeError(w, status, err)
		return
	}
	s.writeAOBatch(w, request.Kind, items, source.Warnings)
}

func decodeSKZRequest(values []string) (generator.SKZRequest, error) {
	var request generator.SKZRequest
	if len(values) != 1 || !strings.HasPrefix(strings.TrimSpace(values[0]), "{") {
		return request, fmt.Errorf("укажите настройки СКЗ в JSON-объекте config")
	}
	decoder := json.NewDecoder(strings.NewReader(values[0]))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return request, fmt.Errorf("неверные настройки СКЗ: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return request, fmt.Errorf("после настроек СКЗ обнаружены лишние данные")
	}
	return request, nil
}

func checkSKZSize(plan *skzmap.Plan) error {
	if plan == nil || len(plan.Groups) == 0 || len(plan.Groups) > maxDocumentPOUs {
		return fmt.Errorf("перекладка СКЗ должна содержать от 1 до %d групп AI/DO", maxDocumentPOUs)
	}
	modules, signals := 0, 0
	for _, group := range plan.Groups {
		modules += len(group.Modules)
		for _, module := range group.Modules {
			signals += len(module.Channels)
		}
	}
	if modules > maxDocumentModules || signals > maxDocumentSignals {
		return fmt.Errorf("перекладка СКЗ превышает пределы %d модулей / %d каналов", maxDocumentModules, maxDocumentSignals)
	}
	return nil
}
