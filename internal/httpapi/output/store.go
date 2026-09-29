// Каталог результатов и единая блокировка операций сохранения.
package output

import (
	"log"

	"sync"
)

type Store struct {
	outputDir string
	logger    *log.Logger
	outputMu  sync.Mutex
}

// New создаёт хранилище результатов для всего HTTP-приложения.
// Получает каталог и logger; общая блокировка исключает пересечение пакетных имён разных обработчиков.
func New(directory string, logger *log.Logger) *Store {
	return &Store{outputDir: directory, logger: logger}
}
