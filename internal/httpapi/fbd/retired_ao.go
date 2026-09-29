// Отказ отключённого маршрута встроенного AO FBD до любых побочных эффектов.
package fbd

import (
	"fmt"

	"net/http"

	"scheme-xml-generator/internal/httpapi/transport"
)

// HandleAOGenerate отклоняет прежний AO-маршрут с жёстко заданным FBD-профилем.
// Возвращает HTTP 410 до чтения карты, allocator и файлов; FBD разрешён только через библиотечные маршруты.
func (s *Service) HandleAOGenerate(w http.ResponseWriter, r *http.Request) {
	transport.WriteError(w, http.StatusGone, fmt.Errorf("FBD создаётся только из подключённой библиотеки. Выберите шаблон на основной странице."))
}
