// Чтение и обновление каталога подключённых XML-библиотек.
package catalog

import (
	"net/http"

	"scheme-xml-generator/internal/httpapi/transport"
)

// HandleTemplates выдаёт интерфейсу текущие типы и шаблоны подключённых библиотек.
// Возвращает снимок Repository.Catalog без изменения файлов или состояния allocator.
func (s *Service) HandleTemplates(w http.ResponseWriter, _ *http.Request) {
	transport.WriteJSON(w, http.StatusOK, s.Repository.Catalog())
}

// HandleRefresh перечитывает библиотеки по явному запросу интерфейса.
// Возвращает обновлённый каталог Repository либо HTTP-ошибку загрузки.
func (s *Service) HandleRefresh(w http.ResponseWriter, _ *http.Request) {
	catalog, err := s.Repository.Refresh()
	if err != nil {
		transport.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	transport.WriteJSON(w, http.StatusOK, catalog)
}
