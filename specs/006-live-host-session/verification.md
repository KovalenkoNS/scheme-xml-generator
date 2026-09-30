# Проверка

Дата: 2026-09-30. Среда: Windows x64.

Воспроизведение по исходникам: `internal/httpapi/workspace/service.go` возвращал постоянный `connected:false, status:contract-pending`; `web/shell/bootstrap.js` игнорировал host, `navigation.js` задавал «Нет подключения». Чтение `HOST_CLIENT_API_URL` и обновление соединения отсутствовали. Владелец отдельно уточнил, что речь об обновлении соединения, а не таблиц; scope до реализации исправлен.

## Действующая приёмка после уточнения владельца

30.09.2026 введён [INT-001](../../.specify/memory/rules.md): проверять соединение исключительно через полное запущенное окно Host-client, действующий Host-server и PostgreSQL, без подмен и прямого подключения модуля к Server/БД. Ниже сохранены прежние фактические результаты; они не доказывают это новое условие. Пакет остаётся `implemented`, а не `verified`.

| Requirement | Check | Result | Evidence |
|---|---|---|---|
| FR-001 | Установка из GitHub и запуск модулем полного Host; реальный Server/PostgreSQL | NOT-RUN | Приёмка всей цепочки готовится в Host `tests/delivery/session`; прежний synthetic Host не засчитывается |
| FR-002 | Вход/выход/смена адреса и обновление в действующей цепочке | NOT-RUN | Нужен один открытый Generator из полного Host с подтверждённым соединением Server с БД |
| FR-003 | Реальные адрес, пользователь и состояние в открытом модуле | NOT-RUN | Результаты браузера с подменённым Host сохранены ниже только как история |
| FR-004 | Потеря и восстановление реальной цепочки, отсутствие устаревшего успеха | NOT-RUN | Чистые проверки модели гонок не заменяют наблюдение запущенных компонентов |
| FR-005 | Предметные каталоги, комментарии и отдельные тесты | PASS | Ручной review ниже; pure unit обнаружения вынесены в `tests/unit/host`, state/timers — `tests/web/host-monitor.test.cjs`; это не проверка сетевого соединения |

## Изолированные результаты до INT-001 — история

| Requirement | Check | Result | Evidence |
|---|---|---|---|
| История FR-001 | Обнаружение Host, самостоятельный запуск и запрет прямого Server | PASS | `go test ./tests/integration/host -count=1`: 5 тестов; loopback/env/без bearer, не следует redirect |
| История FR-002 | Login/logout/address/focus/poll | PASS | `tests/web/verify-host-session.cjs`: 7/7 на реальном EXE + Chromium, один экземпляр и одна страница |
| История FR-003 | Фактическое соединение отдельно от отсутствующего IO-контракта | PASS | Browser подтверждает адрес/пользователя, отсутствие формы входа и по-прежнему заблокированную DB-генерацию; таблиц/обновления версий не добавлено |
| История FR-004 | Смена сессии, поздние ответы, недоступность | PASS | `node --test tests/web/host-monitor.test.cjs`: 4/4; Go: deadline/64KiB/JSON/redirect; browser: потеря Host/Server и автоматическое восстановление |
| История FR-005 | Структура, комментарии, регрессии | PASS | `go run ./tools/rulescheck`: 278 Go файлов, exit0; `node scripts/sdd.cjs check`: 7 пакетов, exit0; ручной review ниже |

Существующие пользовательские библиотеки, output и state не изменяются проверками. Сопоставление IO-колонок генерации и импорт в SCADA не проверяются этим пакетом.

## Доказательства

Browser: `.artifacts/host-session-browser/run-1790763238290-40744/evidence.json`, `connected.png`, `mobile.png`. EXE `.artifacts/live-host-build-a8a39ee6ee274cf689cfbd2cdc1713f3/XmlSchemeGenerator.exe`, SHA256 `a37737584b5160c3c71996470211dcaea9e9371658ac8a72d62ed1448b4bcff6`. Сборка exit0, актуальный файл хэширован; browser runner exit0, 7 checks, errors[]. Тестовый Generator после assertions явно остановлен своим runner (`ownedProcessTermination:true`, SIGTERM); это ожидаемая очистка, не успешное самостоятельное завершение процесса. Снимки 1280×960/430×900 просмотрены: статус, адрес/имя и обновление читаемы, горизонтальной прокрутки нет.

Ручной CODE-001/001-T/002/003 review: `internal/integration/hostclient` разделяет обнаружение endpoint, ограниченный HTTP и безопасный снимок соединения. `internal/httpapi/host` только адаптирует его к HTTP. `web/host` разделяет lifecycle monitor, browser-события и DOM/CSS представление. Shell остаётся точкой подключения; логика генерации/IO туда не добавлена. Тесты находятся в `tests/integration/host` и `tests/web`; комментарии объясняют компонент, вход/эффект и ограничения. Секреты и сетевые трассировки в UI не возвращаются. Применены SDD-001/003/004, CODE-001/001-T/002/003, ARCH-001, DATA-001, UI-001, XML-001/005.

Смежная ошибка Host воспроизведена отдельно: поздний `/auth/me` фоновый probe после logout восстанавливал прежний Connected/Authenticated. Host receipt `.artifacts/session-race-2e57b97c2fcf479291e834145fabd262` — исходный FAIL; guarded revision соединения и авторизации исправляет гонку. Две регрессии — logout и новый login при позднем probe. Host full Go `.artifacts/go-overlay/run-yqsQhz/completion.json`: 133 ожидаемых теста/39 пакетов, список/тест exit0, missing[], 3 launcher SKIP из-за отсутствия sh в PATH. Это не скрывает первоначальный FAIL; окончательные общие проверки принадлежат Host011.

Сквозная проверка опубликованных поставок по INT-001 ещё NOT-RUN: исторический browser использовал реальный Generator и синтетический HTTP Host. В Host `tests/delivery/session/` готовится запуск полного окна Host, установка опубликованного Generator из GitHub и работа через реальный Server/PostgreSQL. Новые synthetic Host/Server-сценарии удаляются из активной приёмки; чистые unit-проверки без сети остаются проверками отдельных функций. Статус пакета оставлен implemented до окончательной поставки и проверки всей цепочки.
