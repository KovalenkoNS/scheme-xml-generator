// Подтверждённые аппаратные интерфейсы шаблона по свободным портам и связям.
package library

import (
	"strings"
)

type ioEndpoint struct {
	id   string
	port string
}

// ioCapabilities определяет доступные аппаратные связи AI/AO/DI/DO по структуре библиотечного шаблона.
// Проверяет подтверждённые типы и свободные порты; возвращает только допустимые профили привязки.
func ioCapabilities(template *Template) []string {
	if template == nil {
		return []string{}
	}
	incoming := make(map[ioEndpoint]bool)
	outgoing := make(map[ioEndpoint]bool)
	for _, primitive := range template.Contents.Primitives {
		if !IsLinkType(primitive.ObjectType) {
			continue
		}
		params := ParseParams(primitive.Params)
		if value, ok := parseEndpointIdentity(params["FP"]); ok {
			outgoing[value] = true
		}
		if value, ok := parseEndpointIdentity(params["LP"]); ok {
			incoming[value] = true
		}
	}

	capabilities := make([]string, 0, 4)
	aiCandidates := 0
	aoCandidates := 0
	digitalBlocks := make([]string, 0)
	for _, primitive := range template.Contents.Primitives {
		id := strings.TrimSpace(primitive.ID)
		switch {
		case primitive.ObjectType == "37" && strings.TrimSpace(primitive.ISAObjectID) == "17480":
			if !incoming[ioEndpoint{id: id, port: "xin"}] && !incoming[ioEndpoint{id: id, port: "xs"}] {
				aiCandidates++
			}
		case primitive.ObjectType == "36" && strings.TrimSpace(primitive.ISAObjectID) == "791" &&
			strings.EqualFold(strings.TrimSpace(primitive.TypeName), "REAL_TO_DINT"):
			if !outgoing[ioEndpoint{id: id, port: "result"}] {
				aoCandidates++
			}
		case primitive.ObjectType == "31" && strings.TrimSpace(primitive.CardID) != "" && strings.TrimSpace(primitive.CardID) != "0":
			params := ParseParams(primitive.Params)
			if Int(params["CI"], 0) == 1 && Int(params["CO"], 0) == 1 {
				digitalBlocks = append(digitalBlocks, id)
			}
		}
	}
	if aiCandidates == 1 {
		capabilities = append(capabilities, "AI")
	}
	if aoCandidates == 1 {
		capabilities = append(capabilities, "AO")
	}
	diCandidates, doCandidates := 0, 0
	for _, id := range digitalBlocks {
		if !incoming[ioEndpoint{id: id, port: "0"}] {
			diCandidates++
		}
		if !outgoing[ioEndpoint{id: id, port: "0"}] {
			doCandidates++
		}
	}
	if diCandidates == 1 {
		capabilities = append(capabilities, "DI")
	}
	if doCandidates == 1 {
		capabilities = append(capabilities, "DO")
	}
	return capabilities
}

// parseEndpointIdentity извлекает идентичность порта из параметра FP/LP библиотечной связи.
// Возвращает ID блока и нормализованное имя порта либо false при неверной четырёхчастной записи.
func parseEndpointIdentity(raw string) (ioEndpoint, bool) {
	parts := strings.SplitN(strings.TrimSpace(raw), "|", 4)
	if len(parts) != 4 {
		return ioEndpoint{}, false
	}
	return ioEndpoint{id: strings.TrimSpace(parts[0]), port: strings.ToLower(strings.TrimSpace(parts[2]))}, true
}
