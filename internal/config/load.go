// Чтение config.json с дополнением значений по умолчанию и проверкой порта.
package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// Load читает config.json при запуске приложения поверх значений Default.
// Возвращает действующие настройки либо ошибку JSON/порта; отсутствие файла сохраняет defaults.
func Load(path string) (Config, error) {
	result := Default()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("прочитать config.json: %w", err)
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, fmt.Errorf("разобрать config.json: %w", err)
	}
	if result.ListenAddress == "" {
		result.ListenAddress = "127.0.0.1"
	}
	if result.Port < 1 || result.Port > 65535 {
		return result, fmt.Errorf("port должен быть в диапазоне 1..65535")
	}
	return result, nil
}
