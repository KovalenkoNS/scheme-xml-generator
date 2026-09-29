// Каталог расположения библиотек и синхронизация доступа к индексам.
package library

import (
	"sync"
)

type Repository struct {
	directory string
	refreshMu sync.Mutex
	mu        sync.RWMutex
	libraries map[string]*LoadedLibrary
	templates map[string]*TemplateRef
	catalog   Catalog
}

// NewRepository создаёт индекс библиотек для выбранного каталога приложения.
// Возвращает Repository без чтения XML; загрузка выполняется явно через Refresh или Import.
func NewRepository(directory string) *Repository {
	return &Repository{directory: directory}
}
