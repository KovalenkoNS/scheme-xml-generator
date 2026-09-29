// Метаданные подтверждённого профиля и доступности сценариев.
package ioao

import (
	"net/http"
	"scheme-xml-generator/internal/generator"

	"scheme-xml-generator/internal/httpapi/transport"
)

// HandleAOProfile выдаёт интерфейсу профиль совместимого AO-маршрута.
// Возвращает действующий контекст и ограничения образца, не меняя общие настройки генератора.
func (s *Service) HandleAOProfile(w http.ResponseWriter, _ *http.Request) {
	transport.WriteJSON(w, http.StatusOK, map[string]any{
		"context":      generator.DefaultAOContext(),
		"fbdAvailable": false,
		"fbdRoute":     "/api/generate",
		"description":  "Генерация FBD по встроенному AO-профилю отключена; выберите подключённую библиотеку. Исторический профиль для чтения карты и ST: AN_v1 по A11_00_example.xml: отдельный XML на каждый FCS, POU AO_A11, AO_A12 и далее; четыре позиции на модуль, свободные каналы заполняются резервами. Повторный тег внутри FCS оставляет пустое место без блока и заглушки; основной вызов сохраняется в Main_module, если он указан в карте. Каждый XML импортируется в соответствующий ПЛК. Числовые ID переназначает SCADA; физические связи не создаются.",
	})
}
