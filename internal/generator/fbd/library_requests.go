// Подготовка библиотечных запросов FBD разрешает экземпляры без встроенного физического графа.
package fbd

import (
	"fmt"
	"scheme-xml-generator/internal/generator/contracts"
	"strings"
)

// NormalizePOURequests проверяет выбранные библиотечные экземпляры до поиска шаблонов и выделения ID.
// Старый io.modules отклоняется и при прямом вызове ядра; физический DO имеет отдельный библиотечный запрос.
func NormalizePOURequests(requests []contracts.POURequest) ([]contracts.POURequest, error) {
	result := make([]contracts.POURequest, len(requests))
	for index, original := range requests {
		if original.IO != nil {
			return nil, fmt.Errorf("POU %d: io.modules отключён; требуется библиотечный план", index+1)
		}
		request := clonePOURequest(original)
		request.DefaultTemplateKey = strings.TrimSpace(request.DefaultTemplateKey)
		for signalIndex := range request.Signals {
			request.Signals[signalIndex] = normalizeSignalRequest(request.Signals[signalIndex])
			if request.Signals[signalIndex].TemplateKey == "" {
				request.Signals[signalIndex].TemplateKey = request.DefaultTemplateKey
			}
			if request.Signals[signalIndex].TemplateKey == "" {
				return nil, fmt.Errorf("POU %d, сигнал %d: библиотечный templateKey не задан", index+1, signalIndex+1)
			}
		}
		result[index] = request
	}
	return result, nil
}

// POUSignals возвращает независимый список экземпляров POU с выбранным шаблоном по умолчанию.
// Встроенная аппаратная схема из io.modules здесь не создаётся.
func POUSignals(pou contracts.POURequest) []contracts.SignalRequest {
	result := append([]contracts.SignalRequest(nil), pou.Signals...)
	for index := range result {
		result[index] = normalizeSignalRequest(result[index])
		if result[index].TemplateKey == "" {
			result[index].TemplateKey = strings.TrimSpace(pou.DefaultTemplateKey)
		}
	}
	return result
}

// normalizeResolvedPOUs закрепляет каждый экземпляр за точным снимком подключённой библиотеки.
// Инверсия изменяет независимую копию разрешённого графа; исходная библиотека не меняется.
func normalizeResolvedPOUs(pous []contracts.ResolvedPOU) ([]contracts.ResolvedPOU, error) {
	requests := make([]contracts.POURequest, len(pous))
	for index, pou := range pous {
		request := clonePOURequest(pou.Request)
		if len(request.Signals) == 0 && len(pou.Signals) != 0 {
			request.Signals = make([]contracts.SignalRequest, len(pou.Signals))
			for signalIndex, signal := range pou.Signals {
				request.Signals[signalIndex] = signal.Request
				if request.Signals[signalIndex].TemplateKey == "" && signal.Ref != nil {
					request.Signals[signalIndex].TemplateKey = signal.Ref.Key
				}
			}
		}
		requests[index] = request
	}
	normalized, err := NormalizePOURequests(requests)
	if err != nil {
		return nil, err
	}
	result := make([]contracts.ResolvedPOU, len(pous))
	for index, pou := range pous {
		result[index] = pou
		result[index].Request = normalized[index]
		effective := POUSignals(normalized[index])
		if len(effective) != len(pou.Signals) {
			return nil, fmt.Errorf("POU %d: разрешено %d шаблонов для %d сигналов", index+1, len(pou.Signals), len(effective))
		}
		result[index].Signals = make([]contracts.ResolvedSignal, len(pou.Signals))
		for signalIndex, signal := range pou.Signals {
			if signal.Ref == nil || signal.Ref.Template == nil || signal.Ref.Library == nil {
				return nil, fmt.Errorf("POU %d, сигнал %d: шаблон не разрешён", index+1, signalIndex+1)
			}
			if effective[signalIndex].TemplateKey != signal.Ref.Key {
				return nil, fmt.Errorf("POU %d, сигнал %d: разрешён шаблон %q вместо %q", index+1, signalIndex+1, signal.Ref.Key, effective[signalIndex].TemplateKey)
			}
			ref := signal.Ref
			if effective[signalIndex].Invert {
				ref, err = invertD32Template(ref)
				if err != nil {
					return nil, fmt.Errorf("POU %d, сигнал %d: %w", index+1, signalIndex+1, err)
				}
			}
			result[index].Signals[signalIndex] = contracts.ResolvedSignal{Request: effective[signalIndex], Ref: ref}
		}
	}
	return result, nil
}

// clonePOURequest копирует изменяемые списки экземпляров перед нормализацией FBD.
// Не поддерживаемый IO сохраняется только для явного отказа, без копирования его реализации.
func clonePOURequest(request contracts.POURequest) contracts.POURequest {
	result := request
	result.Signals = append([]contracts.SignalRequest(nil), request.Signals...)
	return result
}

// normalizeSignalRequest удаляет незначащие пробелы из параметров библиотечного экземпляра.
// Возвращает копию полей запроса без изменения выбранного шаблона.
func normalizeSignalRequest(signal contracts.SignalRequest) contracts.SignalRequest {
	signal.TemplateKey = strings.TrimSpace(signal.TemplateKey)
	signal.ObjectName = strings.TrimSpace(signal.ObjectName)
	signal.NameMode = strings.TrimSpace(signal.NameMode)
	signal.Description = strings.TrimSpace(signal.Description)
	signal.ClusterPath = strings.TrimSpace(signal.ClusterPath)
	return signal
}
