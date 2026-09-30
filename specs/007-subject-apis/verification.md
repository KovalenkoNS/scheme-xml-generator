# Проверка

| Requirement | Check | Result | Evidence |
|---|---|---|---|
| FR-001 | Прямые предметные зависимости и отсутствие facade | PASS | appserver собирает отдельные FBD/ST/HMI; workspace получает только Config и Host. facade.go удалён. Проверки границ запрещают общий facade и межпредметные зависимости HTTP. |
| FR-002 | Единственные определения контрактов по владельцам | PASS | contracts и generator/controller удалены; fbd/request, st/assignment, program, identity, artifact и domain/controller — единственные определения без aliases. Полный Go-набор проверяет сохранение API/XML. |
| FR-003 | Сохранение всех проверок и актуальная карта файлов | PASS | Полный Go, Node21, browser12 AI/AO/DI/DO и CODE gate278 PASS; предметная карта обновлена. Файловая карта Host собирается из реальных исходников. |

30.09.2026, Windows x64. Правила SDD-001/003/004, CODE-001/001-T/002/003, ARCH-001, XML-002/005/006 применены: HTTP связывает предметные операции, модель CPU не зависит от генерации; запросы FBD не содержат ST-планы. Каждый новый файл и функция имеют краткие комментарии о владельце, данных и эффекте. Тесты остаются в tests, overlay добавляет только тесты к настоящим пакетам.

- `go test -json ./... -count=1`: `.artifacts/subject-check-OSbzZ4/receipt.json`, exit0, signal/error=null. 84 внешних теста, включая драйвер 16 whitebox-пакетов/214 деклараций. Единственный внутренний Skip — явный инструмент перезаписи эталона TestRebuildReferenceProfile; эталоны не переписывались. Проверки генерации выполнены.
- Предыдущий запуск `.artifacts/subject-check-qAviVG` завершился exit1: тестовый alias HMI затенял локальную переменную и девять whitebox-обращений использовали удалённое поле generator.Config. Исправлены обращения к новым владельцам; проверки и утверждения сохранены. Блокировки антивирусом в этом запуске не обнаружены.
- `node --test tests/sdd/sdd.test.cjs tests/web/workspace-model.test.cjs tests/preview/parser.test.cjs tests/web/host-monitor.test.cjs`: 21 PASS, 0 FAIL/SKIP, exit0.
- `GENERATOR_EXE=.artifacts/release-20260930-fhv4lp/XmlSchemeGenerator.exe; node tests/web/verify-directions.cjs`: 12 PASS, `.artifacts/directions-browser/run-1790763702443-20420/evidence.json`. AI/AO/DI/DO × библиотечный FBD, поддержанный ST и HMI715; проверены выбранный ПЛК, фактический сохранённый XML и его просмотр. EXE SHA256 `f80a3c1ff89b8e158a80fe16d8fa924c871d116a747f071379595f7094a980dc`. Отказы неподдержанных промежуточных preview API записаны отдельно; ошибок JS нет.
- `go run ./tools/rulescheck`: 278 Go-файлов PASS. `node scripts/sdd.cjs check`: 7 пакетов PASS. Добавлены отрицательные проверки повторного facade/contracts и неправильной зависимости предметного HTTP.

Импорт и исполнение XML в SCADA не проверялись. Матрица подтверждает поддержанные профили, не все сочетания оборудования. Совместная поставка соединения через GitHub описана отдельно в006 и Host011.

После нового требования INT-001 проверки соединения с подменой Host удалены; они не являются приёмкой соединения. Итоговый `go test -json ./... -count=1` от30.09: `.artifacts/final-source-check-nvpSNs/receipt.json`, exit0/signal=null/error=null; 80 внешних PASS, 0 FAIL/SKIP, тот же whitebox-набор и только намеренно пропущенная перезапись эталона. В `tests/unit/host` проверяется лишь валидация отсутствующего/недопустимого адреса без сети. Проверка реальной цепочки с БД остаётся в006.

Окончательная поставка source `e4c6c25a55b07600f82efc63f2584ee2db65f001`, GitHub `v2026.09.30`: EXE SHA256 `6957fce6b0ca9c199b25bd2b8a890b861e726186aa6fbd51a0682b057bc8851c`, ZIP `2d0c625dcd4cf0bc60c1a730f2fb0b1aa0ce1eb5ce465802b8ee47853bbf4d2c`. Go buildinfo подтверждает тот же commit и vcs.modified=false. На этих байтах матрица повторена: `.artifacts/directions-browser/run-1790764276790-43336/evidence.json`, 12 PASS, errors[]. Архив проверен по двум разрешённым элементам EXE/README; пользовательских файлов в нём нет.

Финальная карта Host `v2026.09.30.3` включает532 файла Generator; полная сверка четырёх деревьев PASS. Соединение окончательных поставок принято отдельно через полное окно Host, действующий Server и PostgreSQL по006/INT-001. Локальный standalone EXE заменён теми же проверенными байтами с сохранением прежнего в `.artifacts/standalone-replacement-8e7a950e301b4671bfc60f98677f6b59`; конфигурация, библиотеки, состояние и результаты не менялись.
