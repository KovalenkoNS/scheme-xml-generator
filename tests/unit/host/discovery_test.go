// Чистая проверка недопустимого или отсутствующего адреса Host; сетевые соединения не создаются.
package hostclient_test

import (
	"context"
	"scheme-xml-generator/internal/integration/hostclient"
	"testing"
)

// TestAbsentOrInvalidHostAddress проверяет только отказ валидации до сетевого запроса.
// Это unit-проверка входа, а не доказательство соединения модулей или доступности Server/БД.
func TestAbsentOrInvalidHostAddress(t *testing.T) {
	for _, address := range []string{"https://example.com", "http://example.com:8080", "http://127.0.0.1:8080/private", "http://user:secret@127.0.0.1:8080", "http://127.0.0.1:8080?secret=x", "http://127.0.0.1:8080#part", "http://127.0.0.1"} {
		if got := hostclient.New(address).Session(context.Background()); got.Status != "invalid-host" || got.Available {
			t.Errorf("accepted invalid endpoint %q: %+v", address, got)
		}
	}
	t.Setenv(hostclient.APIEnvironment, "")
	if got := hostclient.FromEnvironment().Session(context.Background()); got.Status != "standalone" || got.Connected {
		t.Fatalf("absent env: %+v", got)
	}
}
