# Проверка

Дата: 2026-09-30. Среда: Windows x64.

Воспроизведение по исходникам: `internal/httpapi/workspace/service.go` возвращал постоянный `connected:false, status:contract-pending`; `web/shell/bootstrap.js` игнорировал host, `navigation.js` задавал «Нет подключения». Чтение `HOST_CLIENT_API_URL` и обновление соединения отсутствовали. Владелец отдельно уточнил, что речь об обновлении соединения, а не таблиц; scope до реализации исправлен.

## Действующая приёмка после уточнения владельца

30.09.2026 введён [INT-001](../../.specify/memory/rules.md): проверять соединение исключительно через полное запущенное окно Host-client, действующий Host-server и PostgreSQL, без подмен и прямого подключения модуля к Server/БД. Финальная приёмка выполнена 30.09.2026 в 10:45:05–10:45:19 UTC, пакет `verified`. Исторические проверки с подменой Host ниже сохранены отдельно и не служат основанием этого статуса.

| Requirement | Check | Result | Evidence |
|---|---|---|---|
| FR-001 | Установка из GitHub и запуск модулем полного Host; реальный Server/PostgreSQL | PASS | `run-XnXjm5/evidence.json`: полный Host сам скачал опубликованный Generator, сверил SHA и запустил модуль с унаследованным адресом своего API; отдельного входа и прямого подключения к Server/БД в модуле нет |
| FR-002 | Вход/выход/смена адреса и обновление в действующей цепочке | PASS | Тот же процесс и страница Generator отражают logout/login автоматически, отказ после смены адреса — при ручном обновлении, восстановление — при возврате фокуса; `performance.timeOrigin` неизменён |
| FR-003 | Реальные адрес, пользователь и состояние в открытом модуле | PASS | Проверены точные адрес Server и имя текущего пользователя, статус «Подключено», отсутствие формы входа; недоступная генерация по IO-данным обозначена отдельно. Полный снимок 1280×960 просмотрен |
| FR-004 | Потеря и восстановление реальной цепочки, отсутствие устаревшего успеха | PASS | Отрицательный сценарий меняет адрес в самом Host на закрытый порт: прежний успех сменяется «Сервер недоступен», возврат адреса и вход восстанавливают соединение. Защита от поздних ответов дополнительно проверена чистой моделью monitor, 4/4; эти unit-проверки не выданы за сетевую приёмку |
| FR-005 | Предметные каталоги, комментарии и отдельные тесты | PASS | Ручной review ниже; pure unit обнаружения в `tests/unit/host`, state/timers в `tests/web/host-monitor.test.cjs`; полноцепочечный runner отдельно в Host `tests/delivery/session`. Финальная общая Go-проверка после удаления synthetic тестов exit0 |

## Финальные поставки и маршрут

Основание — Host-client `.artifacts/delivered-session/run-XnXjm5/evidence.json`, PASS 6; запускающий процесс завершился exit0. Полное окно Host PID41720 использовало свой локальный API `http://127.0.0.1:53542`. Вход выполнялся через настоящий консольный интерфейс Host; пароль считывался только в память, ввод маскировался. Generator устанавливался через `/api/catalog/xml-generator/install` и запускался через `/api/projects/xml-generator/run`. Адрес API Host, токен либо путь локальной сборки модуля драйвер не задавал. Режим Electron `RUN_AS_NODE` не использовался.

Проверенная поставка Host собрана из `7b1a84a035189fc09f80149e658e4ef1c6c4c04e`, каталог `.artifacts/release-20260930-live-workspace-1342/unpacked`. SHA256 файлов:

| Файл | SHA256 |
|---|---|
| `Host-client.exe` | `7dc5834a6d76cc07918a1836489bbc777e48a6cea3194c44c457b4c13b511609` |
| `resources/native/host-client-core.dll` | `6b356145b8e6052516b77f2779f4fa91a6d5028d16c4a4a9b48435798f13f761` |
| `resources/native/host-client-bridge.node` | `34989c6fb2e42cd1aa487dd1e2f37669190d1520a981b0cff59c6c52dab81eb2` |

Generator получен Host из публичного `KovalenkoNS/scheme-xml-generator`, latest на момент проверки — `v2026.09.30`, исходники `e4c6c25a55b07600f82efc63f2584ee2db65f001`; SHA256 установленного `XmlSchemeGenerator.exe` — `6957fce6b0ca9c199b25bd2b8a890b861e726186aa6fbd51a0682b057bc8851c`. Один event запуска оставался активным при всех переходах соединения; перезагрузки страницы не было. В завершение Host остановил свой модуль, listener закрылся, `moduleStopped:true`; окно Host и браузер наблюдения завершились с code0, signal/error null, без принудительного завершения.

Доказательство БД получено только запросами запущенного Host: `/api/server/ping` подтвердил connected/authenticated, `/api/database/tables` вернул 108 таблиц до и после сценария. Сам runner напрямую к Server не обращался. Реальный Server — `http://127.0.0.1:8090`; содержимое строк не записывалось в evidence и не изменялось. Отрицательный сценарий использовал закрытый адрес только в изолированном профиле Host; Server и PostgreSQL оставались запущены. Это проверяет соединение, но не реализует получение/сопоставление IO-записей самим Generator.

`host-connected.png` и `generator-connected.png` из того же запуска просмотрены. Для полного снимка Generator использована отдельная обёртка наблюдения Host `.artifacts/delivered-session/viewport-run.cjs`: только CDP viewport 1280×960, без изменений продукта, адресов или окружения модуля. Проверка пяти итоговых JSON/log-файлов не обнаружила введённого пароля.

## Фактические Server и PostgreSQL

Host `.artifacts/delivered-session/run-XnXjm5/administrative-basis.json` отдельно фиксирует административное чтение identity действующих контейнеров; это не альтернативный канал проверки соединения модуля. Команды `docker inspect`, чтения SHA256 исполняемого Server и версии PostgreSQL завершились code0, signal/error null. Идентичности до и после приёмки совпали, оба контейнера healthy:

| Компонент | Фактическая идентичность |
|---|---|
| Server container | `92dce07f22095728887d1b8166310499aeffa4e4f7ab700f1c00ea6e95628405` |
| Server image | `sha256:5d052838d92d3cb8711abc8e23066cdc8b51db00313858051eef0e3d24e3d6bf` |
| `/usr/local/bin/host-server` | SHA256 `57134d7ce290368dadaecee921c04ceac224029dfe2c9e96792ab75ff501c9f4` |
| PostgreSQL container | `1af5c3ffebba8bf0c9bc43aff894a075de7d91627d7893925d219589282c5b82` |
| PostgreSQL image | `sha256:cf78e76683b9ca8c5733cbbdce6c9262b45b6767934dd0a95e671f9a0fc20685` |
| PostgreSQL version | `16.15` |

В receipt также записаны SHA256 локальных файлов миграций `001_initial.sql` (`31392546cb58cca59d3f0d23fcfa2a66a098c98ca28d1de8957329010b53c78a`) и `002_user_auth.sql` (`c6af1133d937f8b6542e6848624e65b3f4cfee58bbe78e956c0af61783f6dbe3`). Хэши этих файлов не доказывают версию применённой схемы. Версия `1.2.0-dev` Server не доказывает соответствие исполняемого файла текущему локальному HEAD; такого соответствия пакет не утверждает.

Финальная общая Go-проверка Generator после удаления synthetic connection tests: `.artifacts/final-source-check-nvpSNs/receipt.json`, exit0, signal/error null, 80 внешних PASS, без внешних SKIP; whitebox 16 пакетов/214 тестов, единственный явный SKIP — ручная регенерация reference. Это отдельное доказательство сохранности кода, не замена действующей цепочки.

## Изолированные результаты до INT-001 — история

| Requirement | Check | Result | Evidence |
|---|---|---|---|
| История FR-001 | Обнаружение Host, самостоятельный запуск и запрет прямого Server | PASS | `go test ./tests/integration/host -count=1`: 5 тестов; loopback/env/без bearer, не следует redirect |
| История FR-002 | Login/logout/address/focus/poll | PASS | `tests/web/verify-host-session.cjs`: 7/7 на реальном EXE + Chromium, один экземпляр и одна страница |
| История FR-003 | Фактическое соединение отдельно от отсутствующего IO-контракта | PASS | Browser подтверждает адрес/пользователя, отсутствие формы входа и по-прежнему заблокированную DB-генерацию; таблиц/обновления версий не добавлено |
| История FR-004 | Смена сессии, поздние ответы, недоступность | PASS | `node --test tests/web/host-monitor.test.cjs`: 4/4; Go: deadline/64KiB/JSON/redirect; browser: потеря Host/Server и автоматическое восстановление |
| История FR-005 | Структура, комментарии, регрессии | PASS | `go run ./tools/rulescheck`: 278 Go файлов, exit0; `node scripts/sdd.cjs check`: 7 пакетов, exit0; ручной review ниже |

Существующие пользовательские библиотеки, output и state не изменяются проверками. Сопоставление IO-колонок генерации и импорт в SCADA не проверяются этим пакетом.

## Доказательства исторической диагностики

Browser: `.artifacts/host-session-browser/run-1790763238290-40744/evidence.json`, `connected.png`, `mobile.png`. EXE `.artifacts/live-host-build-a8a39ee6ee274cf689cfbd2cdc1713f3/XmlSchemeGenerator.exe`, SHA256 `a37737584b5160c3c71996470211dcaea9e9371658ac8a72d62ed1448b4bcff6`. Сборка exit0, актуальный файл хэширован; browser runner exit0, 7 checks, errors[]. Тестовый Generator после assertions явно остановлен своим runner (`ownedProcessTermination:true`, SIGTERM); это ожидаемая очистка, не успешное самостоятельное завершение процесса. Снимки 1280×960/430×900 просмотрены: статус, адрес/имя и обновление читаемы, горизонтальной прокрутки нет.

Ручной CODE-001/001-T/002/003 review: `internal/integration/hostclient` разделяет обнаружение endpoint, ограниченный HTTP и безопасный снимок соединения. `internal/httpapi/host` только адаптирует его к HTTP. `web/host` разделяет lifecycle monitor, browser-события и DOM/CSS представление. Shell остаётся точкой подключения; логика генерации/IO туда не добавлена. Исторические сетевые тесты `tests/integration/host` удалены из активного набора по INT-001; текущие тесты находятся в `tests/unit/host` и `tests/web`, полноценный сетевой сценарий — в Host `tests/delivery/session`. Комментарии объясняют компонент, вход/эффект и ограничения. Секреты и сетевые трассировки в UI не возвращаются. Применены SDD-001/003/004, CODE-001/001-T/002/003, ARCH-001, DATA-001, INT-001, UI-001, XML-001/005.

Смежная ошибка Host воспроизведена отдельно: поздний `/auth/me` фоновый probe после logout восстанавливал прежний Connected/Authenticated. Host receipt `.artifacts/session-race-2e57b97c2fcf479291e834145fabd262` — исходный FAIL; guarded revision соединения и авторизации исправляет гонку. Две регрессии — logout и новый login при позднем probe. Host full Go `.artifacts/go-overlay/run-yqsQhz/completion.json`: 133 ожидаемых теста/39 пакетов, список/тест exit0, missing[], 3 launcher SKIP из-за отсутствия sh в PATH. Это не скрывает первоначальный FAIL; окончательные общие проверки принадлежат Host011.

Исторический browser использовал реальный Generator и синтетический HTTP Host; он не закрывает INT-001. Такие сценарии удалены из активной приёмки, чистые unit-проверки без сети остаются проверками отдельных функций. Текущий статус `verified` основан только на описанной выше реальной поставке и приёмке `run-XnXjm5`; отсутствие инженерного IO-контракта и отдельная необходимость импорта/исполнения в SCADA сохраняются.
