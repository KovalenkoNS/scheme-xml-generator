// Изолированные временные файлы HTTP-тестов и явный контракт запрета встроенного FBD.
package appserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// isolatedHTTPTemp создаёт каталог входов/результатов, не связанный с пользовательскими output/state.
// После теста повторяет удаление при кратком Windows-lock до двух секунд; стойкий сбой остаётся ошибкой теста.
func isolatedHTTPTemp(t *testing.T) string {
	t.Helper()
	directory, err := os.MkdirTemp("", "generator-http-")
	if err != nil {
		t.Fatal(err)
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		t.Fatal(err)
	}
	base, _ := filepath.Abs(os.TempDir())
	if filepath.Dir(absolute) != base || !strings.HasPrefix(filepath.Base(absolute), "generator-http-") {
		t.Fatal("unsafe test cleanup directory")
	}
	t.Cleanup(func() {
		var removeErr error
		for attempt := 0; attempt < 40; attempt++ {
			removeErr = os.RemoveAll(absolute)
			if removeErr == nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Errorf("remove isolated HTTP directory %s: %v", absolute, removeErr)
	})
	return absolute
}

// assertLegacyFBDUnavailable проверяет новый контракт отменённого HTTP-маршрута.
// Требует 410 и понятное предложение подключить библиотеку, без URL фиктивного результата.
func assertLegacyFBDUnavailable(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	var result map[string]any
	if response.Code != http.StatusGone || json.Unmarshal(response.Body.Bytes(), &result) != nil {
		t.Fatalf("legacy FBD status=%d: %s", response.Code, response.Body.String())
	}
	message, _ := result["error"].(string)
	if !strings.Contains(message, "библиотеки") || result["url"] != nil || result["files"] != nil {
		t.Fatalf("invalid legacy FBD error: %s", response.Body.String())
	}
}
