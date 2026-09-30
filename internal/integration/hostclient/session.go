// Снимок доступности Host и серверной сессии; состояние подключения отделено от IO-контракта.
package hostclient

import (
	"context"
	"time"
)

type State struct {
	Available     bool      `json:"available"`
	Connected     bool      `json:"connected"`
	Authenticated bool      `json:"authenticated"`
	Reachable     bool      `json:"reachable"`
	Status        string    `json:"status"`
	Message       string    `json:"message"`
	Server        string    `json:"server,omitempty"`
	Username      string    `json:"username,omitempty"`
	Role          string    `json:"role,omitempty"`
	CheckedAt     time.Time `json:"checkedAt"`
}

type device struct {
	Server     string      `json:"control_server"`
	Connection *connection `json:"connection"`
}

type connection struct {
	Server        string `json:"server"`
	Username      string `json:"username"`
	Role          string `json:"role"`
	Connected     bool   `json:"connected"`
	Authenticated bool   `json:"authenticated"`
	Reachable     bool   `json:"reachable"`
}

// Session каждый раз читает фактический /api/device Host, без собственного кэша входа.
// Возвращает короткое состояние для UI, исключая сырой сетевой ответ и любые секреты.
func (c *Client) Session(ctx context.Context) State {
	state := State{Status: "standalone", Message: "Запустите генератор из Host", CheckedAt: time.Now().UTC()}
	if c == nil {
		return state
	}
	if c.invalid {
		state.Status, state.Message = "invalid-host", "Некорректный адрес Host"
		return state
	}
	if c.address == "" {
		return state
	}
	var response device
	if c.readDevice(ctx, &response) != nil || response.Connection == nil {
		state.Status, state.Message = "host-unavailable", "Host недоступен"
		return state
	}
	current := response.Connection
	state.Available, state.Server = true, response.Server
	if state.Server == "" {
		state.Server = current.Server
	}
	state.Reachable = current.Reachable
	state.Authenticated = current.Authenticated
	if current.Authenticated {
		state.Username, state.Role = current.Username, current.Role
	}
	switch {
	case state.Server == "":
		state.Status, state.Message = "server-unconfigured", "Укажите сервер в Host"
	case !current.Reachable:
		state.Status, state.Message = "server-unavailable", "Сервер недоступен"
	case !current.Authenticated:
		state.Status, state.Message = "authentication-required", "Войдите на сервер в Host"
	case !current.Connected:
		state.Status, state.Message = "server-unavailable", "Соединение с сервером потеряно"
	default:
		state.Status, state.Message, state.Connected = "connected", "Подключено", true
	}
	return state
}
