# Проверка GitHub-доставки

## Исходный отказ

29.09.2026 `main` в GitHub оставалась на `a1752aa0cbc4c13f54429de6f5562eebf6547029`; текущие исправления были только локальными. Репозиторий публичный, авторизованный владелец имеет ADMIN. `/releases/latest` возвращал 404, список releases был пуст; ZIP и SHA256 отсутствовали. Это невыполненный сценарий GitHub-only, а не недоступность локального EXE.

| Requirement | Check | Result | Evidence |
|---|---|---|---|
| FR-001 | Исходники в удалённой main | PASS | Non-force push завершился exit 0; GitHub API подтвердил `3a297c1871aa8adad2c1b7badf3c2ce0bdc03336`. |
| FR-002 | Настоящие GitHub assets | PASS | [Release v2026.09.29](https://github.com/KovalenkoNS/scheme-xml-generator/releases/tag/v2026.09.29), public latest API 200; ZIP и SHA256 доступны без авторизации. |
| FR-003 | Чистая установка через Host | PASS | 6/6 настоящих сетевых сценариев: пустой Host, install, hash/provenance, повтор, запуск и восстановление регистрации. |
| FR-004 | Отдельные удалённые доказательства | PASS | Точные source/tag/asset SHA ниже; предыдущие FAIL и ограничения сохранены. |

Подготовленный кандидат: `.artifacts/css-candidate-20260929-172154/XmlSchemeGenerator.exe`, SHA256 `d199d66a2ae676a3f67fd9d1102aec03e5ee849043bfe7a50878cfe1f00d4a23`. ZIP SHA256 `ddec0ec6e14c8ee4e10cfc38ff99ae10edf470c981cb492d7239b74d978dedae`. Локальные Go/Node/browser/CSS проверки описаны в пакете 004; здесь они не подменяют GitHub-проверку.

## Исправления переносимости перед публикацией

- Межрепозиторные ссылки на соседний `Develop` заменены canonical GitHub URL. Самостоятельный clone не требует расположения соседних репозиториев на машине владельца.
- Проверка индекса обнаружила завершающие пробелы в прежних SDD-файлах и лишние пустые строки EOF в трёх web-файлах и browser verifier. Исправлено только форматирование; этот FAIL сохранён отдельно от последующей проверки.
- Первый чистый `git archive` commit `74fd6a6` завершился **FAIL** на SDD: `.vscode/tasks.json: Missing file`. Причина — прежнее игнорирование всего `.vscode/`. Добавлены только общие `tasks.json` и `extensions.json`; личные настройки редактора остаются исключёнными. После исправления проверяется новый экспорт, а не исходная рабочая папка.
- Финальный релиз будет заново собран из экспортированного commit после исправления переносимости; хеш прежнего CSS-кандидата выше не выдаётся за опубликованный EXE.

## Опубликованный результат

Исходный commit релиза: [3a297c1871aa8adad2c1b7badf3c2ce0bdc03336](https://github.com/KovalenkoNS/scheme-xml-generator/commit/3a297c1871aa8adad2c1b7badf3c2ce0bdc03336). В чистом `git archive` без соседних репозиториев выполнены SDD, `go test ./... -count=1`, `go vet ./...`, 17 Node-тестов и Go build: каждый child status 0, signal/error null. Необязательные внешние библиотеки отсутствуют в этом экспорте; полная проверка с локальными библиотеками отдельно сохранена в 004.

Из этого экспорта собран новый EXE. На нём повторены workspace 8, directions 12 (AI/AO/DI/DO) и preview 8; свежие XML для preview сформированы настоящим генератором. Все дочерние коды 0; доказательства `.artifacts/github-delivery/run-3a297c1/{clean-source-receipt,browser-receipt}.json` и соответствующие журналы.

- Tag: `v2026.09.29`, release ID `399243323`, draft=false, prerelease=false, endpoint latest возвращает этот tag.
- ZIP: `XmlSchemeGenerator-windows-amd64.zip`, 3 615 299 байт, SHA256 `ef66f28f90fe10ef903880f7af6f95fcb8b3417f0f5af6357ac1856ee9bc9b71`.
- EXE: SHA256 `550d2b90def4a0e28c3a51b4901eb1813cf3366476727c4b82bab22be59e0a6e`.
- GitHub asset digest совпал с локальным ZIP. Архив содержит только EXE и README; каждый извлечённый файл побайтово совпал с исходным.
- Корневой EXE и `dist/` заменены этими же опубликованными байтами после проверки; прежние файлы сохранены в `.artifacts/github-delivery/previous-installed-1790693063653`. Receipt: `.artifacts/github-delivery/installed-release.json`.

## Реальный Host → GitHub → модуль

29.09.2026 в 14:44 UTC packaged Host запущен с новым пустым каталогом состояния и пустым адресом Server. Никакой транспортной фикстуры, токена GitHub или подстановки локального пути генератора нет. Public API и assets доступны с HTTP 200.

`POST /api/catalog/xml-generator/install` вернул 200, Host сам скачал release в собственный managed каталог и записал Source=github/Version=v2026.09.29. SHA установленного EXE совпал с опубликованным. Повторная установка сохранила хеши дерева. Настоящий `/api/projects/xml-generator/run` вернул 202; Host наблюдал работающий процесс, health/UI/CSS модуля ответили 200. Тест остановил только свой процесс и подтвердил восстановление регистрации после перезапуска Host.

6/6 PASS; внешний и дочерний процессы завершились кодом 0, signal/error null, stderr пуст. Host receipt: `.artifacts/github-live-acceptance-20260929/run-1790693065995-26964/{parent-receipt,evidence}.json`; core DLL SHA256 `879d1ba0d089f25b44023a6867fc5dc0e5902f0d0ccd3c31bbde2c1bfd5a8a63`. Рабочие Host/Server, учётные данные и инженерные данные не использовались и не изменялись.

Эта проверка подтверждает загрузку/подключение/запуск стандартного модуля. Она не подтверждает пока не реализованный контракт IO между модулем и Host, физические жесты окон Windows или импорт/исполнение XML в SCADA.
