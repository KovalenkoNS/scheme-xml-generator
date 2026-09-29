# Повторное открытие после отклонения владельцем

Дата: 29.09.2026. Прежняя приёмка **отозвана (FAIL)**. Источник — прямое отклонение структуры, пространства имён, дублирования и реакции на антивирус. Исправления ниже не подменяют финальную проверку нового кандидата; до неё пакет остаётся ready.

## Подтверждённые дефекты и исправления

| ID | Правило | Факт при повторном аудите | Исправление / доказательство |
|---|---|---|---|
| R001 | CODE-003 | Общие ST/DO использовали AOMappingContext; HMI всего ПЛК — AODiagnosticContext; пакет любых XML — AoBatchItem/WriteAOBatch | ProgramContext, HMIContext, ControllerResult/BatchResponse, OutputSTDocument; AO default отделён от module/HMI. Регрессия TestModuleAndHMIContextsDoNotInheritAOProject |
| R002 | CODE-003 | Общий ST выдавал temporary:SKZ_ST и SKZ_ST; ошибки называли общий контекст СКЗ | Метаданные assignments:ST:<kind>, MODULE_ASSIGNMENTS_ST; сообщения общих лимитов не содержат СКЗ/ПАЗ |
| R003 | CODE-001/003, XML-002 | Production facade вызывал fixed FBD; DO renderer имел nil-профиль со встроенными типами | Fixed dispatcher/planning/рендеры исключены из production, DO требует библиотечный профиль. TestCoreRejectsRetiredIOBeforeLibraryRendering проверяет AI/AO/DI/DO независимо от HTTP |
| R004 | CODE-003, DOMAIN-001 | ValidateAODestination определял ПАЗ только из CPU850 | exportprofile.ValidateAOPhysicalST сообщает неподтверждённый профиль, не область/отсутствие AO. HTTP-регрессия запрещает прежнее сообщение |
| R005 | CODE-001/003 | Аппаратная ёмкость повторялась в AO/IO адаптерах, HMI, FBD и moduleprofile | Единственный domain/hardware; профиль назначения ST и поддержка экспорта отделены. AOC4HChannels задаёт также размер фиксированного AO HMI-кадра |
| R006 | CODE-001-T/002 | Помощники AO-входов были скопированы в FBD/ST/HMI; комментарии тестов отсутствовали | tests/support/aomap и fixtures; 266 предметных комментариев функций добавлены совместным review. Gate включает production/tests/support, регрессия требует FAIL при пропуске |
| R007 | SDD-003 | Старые тесты были выданы как приёмка всех требований несмотря на нарушения | verified снят. Прежний отчёт сохранён как история; текущие результаты и ограничения записаны отдельно в verification |
| R008 | SDD-003/004 | Ошибка PowerShell-записи SOURCE_LAYOUT была пропущена в отчёте | Подтверждены Kaspersky501 Record867 и exit1 ScriptContainedMaliciousContent в15:39; поздний patch15:41 не отменяет сбой. [execution-incident](execution-incident.md) |
| R009 | CODE-001/003 | Разнесённые web-файлы продолжали изменять общий window.GeneratorWorkspace, завися от порядка загрузки | Явные ES imports/exports, одна точка bootstrap; [browser-module-review](browser-module-review.md) |
| R010 | CODE-001/003 | shell/workspace.css объединяет оболочку, источники, библиотеку, оборудование и настройки/результаты генерации | Исправлено по владельцам компонентов; 162 правила и вычисленные стили совпали, новый кандидат workspace8/preview8 PASS. [CSS review](css-separation-review.md) сохраняет также ошибки проверочного сценария |

HTTP prepareDocument уже отклонял io.modules до генерации с410: обход этого ограничения не установлен. Нарушение R003 означало сохранение второго алгоритма в production, а не доказанную доступность HTTP-маршрута.

Побайтовых копий канонических исходников при начальном SHA256-аудите не найдено. Подтверждённое дублирование касалось обязанностей, констант и помощников. Снятые тестовые утверждения перечислены в [tests/retired](../../tests/retired/README.md): RETIRED/NOT-RUN, без второй production-реализации в tests. Проверки HTTP410 переименованы по фактическому запрету и не выдаются за старое fixed-FBD coverage.

Резервные материалы .artifacts, сторонние Spec Kit-файлы и неизменённые исходные XML/XLSX не являются дополнительными каноническими исходниками. Отклонённая прежняя EXE/ZIP-поставка не объявляется обновлённой без новой сборки и проверки.
