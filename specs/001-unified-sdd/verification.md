# Verification: Единый SDD для XML Scheme Generator

Дата: 2026-09-28. Пакет: [spec.md](spec.md). Формат результатов: `PASS`, `FAIL`, `NOT-RUN`. Статус `implemented` означает подготовленные изменения, но не подменяет фактическую приёмку.

| Requirement | Check | Result | Evidence |
|---|---|---|---|
| FR-001 | Общий процесс, официальный Spec Kit v1.0.12, конституция и четыре файла; `node scripts/sdd.cjs check` | PASS | Проверка завершилась с кодом 0: `SDD check passed: 1 Spec Kit feature(s)`. Официальный `check-prerequisites.ps1 -Json -RequireSpec -RequireTasks -IncludeTasks` завершился с кодом 0 и выбрал текущий пакет. |
| FR-002 | Полнота US1 / FR-001…005 / SC-001…003 / T001…T006 и результатов проверок | PASS | Общий SDD check прошёл; просмотр spec/plan/tasks/verification подтвердил сценарии US1, критерии SC и связь каждого FR с задачей и строкой проверки. |
| FR-003 | Сохранение десяти TASK по SHA256; новые указатели ведут в Spec Kit | PASS | Python 3.14.4 сравнил все 10 файлов с исходными хешами ниже: 10/10 совпадают. AGENTS, глава 07 и журнал задач направляют новую работу в Spec Kit; старые карточки не изменялись. |
| FR-004 | Роли продуктов согласованы в AGENTS, конституции, главах 01/03/06, overview и ADR-0009 | PASS | Сопоставлены все перечисленные документы: Host-client — оркестратор; Host-server — авторизация клиентов и маршрутизация; xml-generator — независимое ПО. Q-INT-001/002/003 и Q-CTX-001/002 оставлены частично отвеченными с явным перечнем неизвестного. |
| FR-005 | Документационные ссылки, обзор изменений, сохранность кода/данных и доказательств SCADA | PASS | Проверены 167 локальных ссылок в 17 авторских документах: отсутствующих целей нет. Обзор внесённых правок: изменены процесс и документация; продуктовый код, пользовательские данные и EXE этой миграцией не редактировались. Ограничения импорта сохранены. |

## Выполненные проверки

Окружение: Windows, PowerShell, Python 3.14.4; команды выполнялись из корня `scheme-xml-generator`.

```powershell
node scripts/sdd.cjs check
powershell -NoProfile -ExecutionPolicy Bypass -File .specify/scripts/powershell/check-prerequisites.ps1 -Json -RequireSpec -RequireTasks -IncludeTasks
Get-ChildItem -LiteralPath docs/tasks -Filter 'TASK-*.md' | Sort-Object Name | Get-FileHash -Algorithm SHA256
```

Официальная проверка вернула `FEATURE_DIR` текущего `specs/001-unified-sdd` и `AVAILABLE_DOCS: ["tasks.md"]`; проверяет наличие обязательных файлов, а не полноту приёмки. Дополнительно Python прочитал ожидаемые SHA256 из таблицы ниже и сравнил их с `hashlib.sha256` содержимого карточек; все совпали. Проверка локальных целей извлекла Markdown-ссылки и HTML `href` из конституции, пакета, AGENTS и обновлённых предметных документов, разрешила пути относительно исходного файла и проверила наличие. Сетевые ссылки и Markdown-якоря не проверялись. Каталог `.kiro` не создавался.

## Исходные хеши исторических карточек

SHA256 рассчитаны до начала изменений этого пакета. Файлы находятся в `docs/tasks/`; карточки TASK-0004…0010 уже существовали в рабочем дереве и не были отслежены Git.

```text
9B33455F6F86A8FCCC5E97B6B5975B51524547176097D4B300E2B83879583BD3  TASK-0001-documentation-baseline.md
49BA0FBE8C4F24A50C6FA5787595BA6AD449B994B9795C7CE47A2DF305E7E66C  TASK-0002-scada-xml-boundary.md
287FAD11B5EBD631C8D24EB5A27EFEDD9949C529404413DD1D49CABA3ED1B544  TASK-0003-git-current-version.md
42BA46FEEF56B2D18C678504ED366428264AC0616F90BC07C1172618D149D5BF  TASK-0004-sync-from-github.md
330F7F513E6FEB251934EEABD6F162298AE61F166A7CB61BEA7C92FA8C0F84D2  TASK-0005-dual-plc-support.md
4BE232B501E990B5C56601A71A6BADE0D65C5EBBAF68E5412E663AC3785B921C  TASK-0006-di850-paz.md
1BF1295AA8AB7B277CE96416FB7945BEE60F5006214ACF8BEF922F98BCCED136  TASK-0007-raw-do-excel.md
759A7D6A5FC2F2FEF5349D801CF1568E95F5A25A966AAB986D36BC9B6A7AA6C0  TASK-0008-do-st-module-references.md
45068FB71199630CD6BDF16C2CB2DE9509C23AA73F06888D7B5D4984084FD714  TASK-0009-do-st-full-channels.md
425F23BB6A935061B2200BDF6220847099DB5DA717C1717BF27C1A2ABF5691F0  TASK-0010-do-native-fbd.md
```

## Ограничения

Это проверка миграции документационного процесса. Она не подтверждает корректность генерации XML, импорт, компиляцию или выполнение в SCADA. Исторические ограничения TASK и частично открытые Q-INT/Q-CTX сохраняются. Коммит, переключение ветки и публикация не выполнялись.

## Итоговая проверка всех проектов (28.09.2026)

- Spec Kit 1.0.12 / Codex / PowerShell установлен. SHA256 общих шаблонов, навыков, скриптов и руководства SDD совпадают во всех четырёх репозиториях. Конституции проектов остаются отдельными.
- `node scripts/sdd.cjs check`: PASS.
- `node --test scripts/sdd.test.cjs`: PASS, 3 теста. Проверены официальное создание спецификации, JSON подготовки плана и задач, определение активного пакета, сохранность файлов и Git-ветки, отклонение некорректного slug и незавершённой приёмки. После окончательного исправления совместимости тесты прошли во всех четырёх репозиториях.
- Локальная поправка `.specify/scripts/powershell/common.ps1` включает UTF-8 для консоли: Windows PowerShell 5.1 больше не заменяет стрелки управляющим байтом `0x1A` в JSON шаблона. Происхождение и поправка записаны в `.specify/upstream.json`.
- Выбор навыков и запуск задач через интерфейс VS Code: NOT-RUN. Файлы навыков и определения задач проверены. Поведение приложений, импорт SCADA и внешние службы этой миграцией не проверялись.
