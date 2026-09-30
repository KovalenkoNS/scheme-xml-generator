// Выбор и проверка библиотечных шаблонов каждого сигнала FBD.
package fbd

import (
	"fmt"
	"scheme-xml-generator/internal/generator/fbd"
	fbdrequest "scheme-xml-generator/internal/generator/fbd/request"
	"scheme-xml-generator/internal/httpapi/limits"
	"scheme-xml-generator/internal/library"
	"strings"
)

// collectTemplateKeys проверяет пределы документа и находит templateKey всех сигналов.
// Получает нормализованный Request, возвращает ключи для одного Repository.ResolveMany без побочных эффектов.
func collectTemplateKeys(request fbdrequest.Request) ([]string, error) {
	keys := make([]string, 0)
	signalCount := 0
	moduleCount := 0
	for pouIndex, pou := range request.POUs {
		if pou.IO != nil {
			moduleCount += len(pou.IO.Modules)
			if moduleCount > limits.MaxDocumentModules {
				return nil, fmt.Errorf("один файл может содержать не более %d физических модулей", limits.MaxDocumentModules)
			}
		}
		signals := fbd.POUSignals(pou)
		if len(signals) == 0 {
			return nil, fmt.Errorf("POU %d не содержит сигналов", pouIndex+1)
		}
		signalCount += len(signals)
		if signalCount > limits.MaxDocumentSignals {
			return nil, fmt.Errorf("один файл может содержать не более %d сигналов", limits.MaxDocumentSignals)
		}
		for signalIndex, signal := range signals {
			key := strings.TrimSpace(signal.TemplateKey)
			if key == "" {
				key = strings.TrimSpace(pou.DefaultTemplateKey)
			}
			if key == "" {
				return nil, fmt.Errorf("POU %d, сигнал %d: шаблон не выбран", pouIndex+1, signalIndex+1)
			}
			keys = append(keys, key)
		}
	}

	return keys, nil
}

// resolvePOUSignals связывает сигналы POU с разрешёнными библиотечными шаблонами.
// Проверяет совместимость каждого уникального шаблона и возвращает входы предметного генератора FBD.
func resolvePOUSignals(request fbdrequest.Request, references map[string]*library.TemplateRef) ([]fbdrequest.ResolvedPOU, error) {
	checked := make(map[string]struct{}, len(references))
	resolved := make([]fbdrequest.ResolvedPOU, 0, len(request.POUs))
	for pouIndex, pou := range request.POUs {
		signals := fbd.POUSignals(pou)
		resolvedPOU := fbdrequest.ResolvedPOU{Request: pou, Signals: make([]fbdrequest.ResolvedSignal, 0, len(signals))}
		for signalIndex, signal := range signals {
			key := strings.TrimSpace(signal.TemplateKey)
			if key == "" {
				key = strings.TrimSpace(pou.DefaultTemplateKey)
			}
			ref := references[key]
			if _, exists := checked[key]; !exists {
				supported, warnings := library.TemplateCompatibility(ref)
				if !supported {
					return nil, fmt.Errorf("POU %d, сигнал %d: шаблон %q несовместим с генератором: %s", pouIndex+1, signalIndex+1, ref.Template.Name, strings.Join(warnings, "; "))
				}
				checked[key] = struct{}{}
			}
			effectiveSignal := signal
			effectiveSignal.TemplateKey = key
			resolvedPOU.Signals = append(resolvedPOU.Signals, fbdrequest.ResolvedSignal{Request: effectiveSignal, Ref: ref})
		}
		resolved = append(resolved, resolvedPOU)
	}

	return resolved, nil
}
