// Package hostclient читает состояние оркестратора через переданный им локальный HTTP API.
package hostclient

import (
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const APIEnvironment = "HOST_CLIENT_API_URL"

type Client struct {
	address string
	invalid bool
	http    *http.Client
}

// FromEnvironment обнаруживает API запустившего приложение Host; отсутствие переменной
// сохраняет независимый локальный режим генератора без поиска чужих серверов.
func FromEnvironment() *Client { return New(os.Getenv(APIEnvironment)) }

// New проверяет loopback-адрес Host и создаёт ограниченный транспорт без proxy/redirect.
// Адрес Server и сессионные секреты не используются для прямых запросов генератора.
func New(address string) *Client {
	client := &Client{http: &http.Client{
		Timeout:       2 * time.Second,
		Transport:     &http.Transport{Proxy: nil, DisableKeepAlives: true},
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}}
	address = strings.TrimSpace(address)
	if address == "" {
		return client
	}
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		client.invalid = true
		return client
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() || u.Port() == "" {
		client.invalid = true
		return client
	}
	client.address = strings.TrimRight(u.String(), "/")
	return client
}
