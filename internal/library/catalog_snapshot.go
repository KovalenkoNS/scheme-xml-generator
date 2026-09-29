// Независимые снимки каталога для HTTP-интерфейса без изменения общих данных.
package library

import (
	"encoding/json"
)

// Catalog выдаёт интерфейсу текущий каталог подключённых библиотек.
// Возвращает глубокую копию под read-lock, чтобы клиентский код не изменил общий индекс.
func (r *Repository) Catalog() Catalog {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return cloneCatalog(r.catalog)
}

// cloneCatalog отделяет выдаваемый HTTP-каталог от внутренних срезов Repository.
// Возвращает глубокую JSON-копию метаданных типов/шаблонов и предупреждений.
func cloneCatalog(catalog Catalog) Catalog {
	data, _ := json.Marshal(catalog)
	var clone Catalog
	_ = json.Unmarshal(data, &clone)
	return clone
}
