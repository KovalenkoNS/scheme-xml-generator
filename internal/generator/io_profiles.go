package generator

import (
	"fmt"
	"strconv"
	"strings"

	"scheme-xml-generator/internal/library"
)

const (
	ioTypeAI = "AI"
	ioTypeAO = "AO"
	ioTypeDI = "DI"
	ioTypeDO = "DO"
)

type ioAssignment struct {
	FlatIndex   int
	ModuleIndex int
	Channel     int
	Type        string
	Module      IOModuleRequest
}

// NormalizePOURequests validates the two supported POU request forms and
// assigns missing hardware module IDs. Module IDs share one document-wide
// namespace and are allocated as the smallest currently free non-negative
// signed 32-bit integers.
func NormalizePOURequests(requests []POURequest) ([]POURequest, error) {
	if len(requests) == 0 {
		return nil, fmt.Errorf("документ должен содержать хотя бы одну POU")
	}

	result := make([]POURequest, len(requests))
	explicitIDs := make(map[int64]string)
	for pouIndex, request := range requests {
		result[pouIndex] = clonePOURequest(request)
		pou := &result[pouIndex]
		pou.Name = strings.TrimSpace(pou.Name)
		pou.DefaultTemplateKey = strings.TrimSpace(pou.DefaultTemplateKey)

		if pou.IO == nil {
			if len(pou.Signals) == 0 {
				return nil, fmt.Errorf("POU %d не содержит сигналов", pouIndex+1)
			}
			for signalIndex := range pou.Signals {
				pou.Signals[signalIndex] = normalizeSignalRequest(pou.Signals[signalIndex])
				if pou.Signals[signalIndex].TemplateKey == "" {
					pou.Signals[signalIndex].TemplateKey = pou.DefaultTemplateKey
				}
				if strings.TrimSpace(pou.Signals[signalIndex].TemplateKey) == "" {
					return nil, fmt.Errorf("POU %d, сигнал %d: templateKey и defaultTemplateKey не заданы", pouIndex+1, signalIndex+1)
				}
			}
			continue
		}
		if len(pou.Signals) != 0 {
			return nil, fmt.Errorf("POU %d смешивает signals и io.modules", pouIndex+1)
		}

		ioType, capacity, err := normalizeIOType(pou.IO.Type)
		if err != nil {
			return nil, fmt.Errorf("POU %d: %w", pouIndex+1, err)
		}
		pou.IO.Type = ioType
		if len(pou.IO.Modules) == 0 {
			return nil, fmt.Errorf("POU %d: io.modules не содержит физических модулей", pouIndex+1)
		}
		for moduleIndex := range pou.IO.Modules {
			module := &pou.IO.Modules[moduleIndex]
			module.BindingPrefix = strings.TrimSpace(module.BindingPrefix)
			module.InstanceName = strings.TrimSpace(module.InstanceName)
			if module.BindingPrefix != "" {
				if err := validateIdentifier(module.BindingPrefix); err != nil {
					return nil, fmt.Errorf("POU %d, модуль %d: bindingPrefix: %w", pouIndex+1, moduleIndex+1, err)
				}
			}
			if module.InstanceName != "" {
				if err := validateIdentifier(module.InstanceName); err != nil {
					return nil, fmt.Errorf("POU %d, модуль %d: instanceName: %w", pouIndex+1, moduleIndex+1, err)
				}
			}
			if len(module.Signals) == 0 {
				return nil, fmt.Errorf("POU %d, модуль %d: список сигналов пуст", pouIndex+1, moduleIndex+1)
			}
			if len(module.Signals) > capacity {
				return nil, fmt.Errorf("POU %d, модуль %d: %s допускает не более %d сигналов", pouIndex+1, moduleIndex+1, ioType, capacity)
			}
			for signalIndex := range module.Signals {
				module.Signals[signalIndex] = normalizeSignalRequest(module.Signals[signalIndex])
				if module.Signals[signalIndex].TemplateKey == "" {
					module.Signals[signalIndex].TemplateKey = pou.DefaultTemplateKey
				}
				if module.Signals[signalIndex].TemplateKey == "" {
					return nil, fmt.Errorf("POU %d, модуль %d, канал %d: templateKey и defaultTemplateKey не заданы", pouIndex+1, moduleIndex+1, signalIndex)
				}
			}
			if module.ID == nil {
				continue
			}
			if *module.ID < 0 || *module.ID > maxTransportID {
				return nil, fmt.Errorf("POU %d, модуль %d: ID физического модуля должен быть в диапазоне 0..%d", pouIndex+1, moduleIndex+1, maxTransportID)
			}
			location := fmt.Sprintf("POU %d, модуль %d", pouIndex+1, moduleIndex+1)
			if previous, exists := explicitIDs[*module.ID]; exists {
				return nil, fmt.Errorf("ID физического модуля %d повторяется: %s и %s", *module.ID, previous, location)
			}
			explicitIDs[*module.ID] = location
		}
	}

	usedIDs := make(map[int64]struct{}, len(explicitIDs))
	for id := range explicitIDs {
		usedIDs[id] = struct{}{}
	}
	nextID := int64(0)
	for pouIndex := range result {
		pou := &result[pouIndex]
		if pou.IO == nil {
			continue
		}
		for moduleIndex := range pou.IO.Modules {
			module := &pou.IO.Modules[moduleIndex]
			if module.ID != nil {
				continue
			}
			for {
				if nextID > maxTransportID {
					return nil, fmt.Errorf("исчерпаны ID физических модулей")
				}
				if _, exists := usedIDs[nextID]; !exists {
					break
				}
				nextID++
			}
			assigned := nextID
			module.ID = &assigned
			usedIDs[assigned] = struct{}{}
			nextID++
		}
	}
	return result, nil
}

// POUSignals returns the effective flat signal order consumed by the resolver
// and generator. An I/O POU is flattened module-by-module and channel-by-
// channel; a signal-level template overrides the POU default.
func POUSignals(pou POURequest) []SignalRequest {
	if pou.IO == nil {
		result := append([]SignalRequest(nil), pou.Signals...)
		for index := range result {
			result[index] = normalizeSignalRequest(result[index])
			if result[index].TemplateKey == "" {
				result[index].TemplateKey = strings.TrimSpace(pou.DefaultTemplateKey)
			}
		}
		return result
	}
	count := 0
	for _, module := range pou.IO.Modules {
		count += len(module.Signals)
	}
	result := make([]SignalRequest, 0, count)
	for _, module := range pou.IO.Modules {
		for _, signal := range module.Signals {
			signal = normalizeSignalRequest(signal)
			if signal.TemplateKey == "" {
				signal.TemplateKey = strings.TrimSpace(pou.DefaultTemplateKey)
			}
			result = append(result, signal)
		}
	}
	return result
}

func normalizeResolvedPOUs(pous []ResolvedPOU) ([]ResolvedPOU, error) {
	requests := make([]POURequest, len(pous))
	for index, pou := range pous {
		request := clonePOURequest(pou.Request)
		// Direct generator callers historically supplied the resolved flat list
		// without mirroring it into POURequest.Signals. Preserve that API.
		if request.IO == nil && len(request.Signals) == 0 && len(pou.Signals) != 0 {
			request.Signals = make([]SignalRequest, len(pou.Signals))
			for signalIndex, signal := range pou.Signals {
				request.Signals[signalIndex] = signal.Request
				if request.Signals[signalIndex].TemplateKey == "" && signal.Ref != nil {
					request.Signals[signalIndex].TemplateKey = signal.Ref.Key
				}
			}
		}
		requests[index] = request
	}
	normalizedRequests, err := NormalizePOURequests(requests)
	if err != nil {
		return nil, err
	}
	result := make([]ResolvedPOU, len(pous))
	for index, pou := range pous {
		result[index] = pou
		result[index].Request = normalizedRequests[index]
		effective := POUSignals(normalizedRequests[index])
		if len(effective) != len(pou.Signals) {
			return nil, fmt.Errorf("POU %d: разрешено %d шаблонов для %d сигналов", index+1, len(pou.Signals), len(effective))
		}
		result[index].Signals = make([]ResolvedSignal, len(pou.Signals))
		for signalIndex, signal := range pou.Signals {
			if signal.Ref == nil || signal.Ref.Template == nil || signal.Ref.Library == nil {
				return nil, fmt.Errorf("POU %d, сигнал %d: шаблон не разрешён", index+1, signalIndex+1)
			}
			if effective[signalIndex].TemplateKey != "" && effective[signalIndex].TemplateKey != signal.Ref.Key {
				return nil, fmt.Errorf("POU %d, сигнал %d: разрешён шаблон %q вместо %q", index+1, signalIndex+1, signal.Ref.Key, effective[signalIndex].TemplateKey)
			}
			result[index].Signals[signalIndex] = ResolvedSignal{Request: effective[signalIndex], Ref: signal.Ref}
		}
	}
	return result, nil
}

func clonePOURequest(request POURequest) POURequest {
	result := request
	result.Signals = append([]SignalRequest(nil), request.Signals...)
	if request.IO != nil {
		ioCopy := *request.IO
		ioCopy.Modules = make([]IOModuleRequest, len(request.IO.Modules))
		for index, module := range request.IO.Modules {
			ioCopy.Modules[index] = module
			ioCopy.Modules[index].Signals = append([]SignalRequest(nil), module.Signals...)
		}
		result.IO = &ioCopy
	}
	return result
}

func normalizeSignalRequest(signal SignalRequest) SignalRequest {
	signal.TemplateKey = strings.TrimSpace(signal.TemplateKey)
	signal.ObjectName = strings.TrimSpace(signal.ObjectName)
	signal.NameMode = strings.TrimSpace(signal.NameMode)
	signal.Description = strings.TrimSpace(signal.Description)
	signal.ClusterPath = strings.TrimSpace(signal.ClusterPath)
	return signal
}

func normalizeIOType(value string) (string, int, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	switch value {
	case ioTypeAI:
		return value, 16, nil
	case ioTypeAO:
		return value, 4, nil
	case ioTypeDI, ioTypeDO:
		return value, 32, nil
	default:
		return "", 0, fmt.Errorf("неизвестный тип физического ввода/вывода %q; ожидается AI, AO, DI или DO", value)
	}
}

func ioAssignmentsForPOU(pou POURequest) []ioAssignment {
	if pou.IO == nil {
		return nil
	}
	result := make([]ioAssignment, 0, len(POUSignals(pou)))
	flatIndex := 0
	for moduleIndex, module := range pou.IO.Modules {
		for channel := range module.Signals {
			result = append(result, ioAssignment{
				FlatIndex: flatIndex, ModuleIndex: moduleIndex, Channel: channel,
				Type: pou.IO.Type, Module: module,
			})
			flatIndex++
		}
	}
	return result
}

func effectiveBindingPrefix(ioType string, module IOModuleRequest) string {
	if value := strings.TrimSpace(module.BindingPrefix); value != "" {
		return value
	}
	prefix := "_IO_IU"
	if ioType == ioTypeAO || ioType == ioTypeDO {
		prefix = "_IO_QU"
	}
	return prefix + strconv.FormatInt(*module.ID, 10)
}

func effectiveModuleInstanceName(ioType string, module IOModuleRequest) string {
	if value := strings.TrimSpace(module.InstanceName); value != "" {
		return value
	}
	return "_IO_" + ioType + "_" + strconv.FormatInt(*module.ID, 10)
}

func ioDeviceName(ioType string) string {
	switch ioType {
	case ioTypeAI:
		return "CAIH16V"
	case ioTypeAO:
		return "CAOH4V"
	case ioTypeDI:
		return "CDI32"
	case ioTypeDO:
		return "CDO32"
	default:
		return ""
	}
}

func validateTemplateIOAnchor(ref *library.TemplateRef, ioType string) error {
	if ref == nil || ref.Template == nil {
		return fmt.Errorf("шаблон отсутствует")
	}
	links := templateEndpointUsage(ref.Template)
	candidates := 0
	for _, primitive := range ref.Template.Contents.Primitives {
		params := library.ParseParams(primitive.Params)
		id := strings.TrimSpace(primitive.ID)
		switch ioType {
		case ioTypeAI:
			if primitive.ObjectType == "37" && strings.TrimSpace(primitive.ISAObjectID) == "17480" &&
				!links.incoming[endpointKey(id, "Xin")] && !links.incoming[endpointKey(id, "Xs")] {
				candidates++
			}
		case ioTypeAO:
			if primitive.ObjectType == "36" && strings.TrimSpace(primitive.ISAObjectID) == "791" &&
				strings.EqualFold(strings.TrimSpace(primitive.TypeName), "REAL_TO_DINT") &&
				!links.outgoing[endpointKey(id, "Result")] {
				candidates++
			}
		case ioTypeDI:
			if isDigitalAnchorPrimitive(primitive, params) && !links.incoming[endpointKey(id, "0")] {
				candidates++
			}
		case ioTypeDO:
			if isDigitalAnchorPrimitive(primitive, params) && !links.outgoing[endpointKey(id, "0")] {
				candidates++
			}
		}
	}
	if candidates != 1 {
		return fmt.Errorf("шаблон %q несовместим с %s: найдено %d свободных точек физической привязки, требуется ровно одна", ref.Template.Name, ioType, candidates)
	}
	return nil
}

func isDigitalAnchorPrimitive(primitive library.Primitive, params map[string]string) bool {
	cardID := strings.TrimSpace(primitive.CardID)
	return primitive.ObjectType == "31" && cardID != "" && cardID != "0" &&
		library.Int(params["CI"], 0) == 1 && library.Int(params["CO"], 0) == 1
}

type endpointUsage struct {
	incoming map[string]bool
	outgoing map[string]bool
}

func templateEndpointUsage(template *library.Template) endpointUsage {
	result := endpointUsage{incoming: make(map[string]bool), outgoing: make(map[string]bool)}
	for _, primitive := range template.Contents.Primitives {
		if !library.IsLinkType(primitive.ObjectType) {
			continue
		}
		params := library.ParseParams(primitive.Params)
		if id, pin, ok := endpointIdentity(params["FP"]); ok {
			result.outgoing[endpointKey(id, pin)] = true
		}
		if id, pin, ok := endpointIdentity(params["LP"]); ok {
			result.incoming[endpointKey(id, pin)] = true
		}
	}
	return result
}

func endpointIdentity(value string) (string, string, bool) {
	parts := strings.SplitN(strings.TrimSpace(value), "|", 4)
	if len(parts) != 4 {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[2]), true
}

func endpointKey(id, pin string) string {
	return strings.TrimSpace(id) + "\x00" + strings.ToUpper(strings.TrimSpace(pin))
}
