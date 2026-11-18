# Редактор и выполнение скриптов — 2026-11-14

Реализован сценарий: входящий пакет → Копировать → requests/curl/pwntools → редактирование кода → копирование или реальное выполнение Python/Bash в отдельном контейнере. Это выполнение произвольного текста редактора, а не HTTP replay API. Интерактивный stdin/PTY не реализован; stdout/stderr поступают снимками во время запуска.

## Проверки

- Генератор: `node --test tests/browser/request-export.test.mjs` — 10 PASS, включая отправку сгенерированных requests/curl/pwntools в локальные HTTP/TCP/UDP recording servers, байты/Unicode/BOM, pipeline/chunked, неполные данные, escaping, повторяющиеся заголовки и компиляцию Python.
- Backend: `go test -race ./internal/runner ./internal/api ./internal/dockerui ./cmd/discoverer` — PASS. Проверены ограничения, отмена во время создания, timeout/kill, ограничение вывода, concurrency, startup cleanup, аутентификация, Origin и фиксированные proxy-пути.
- `TestStreamDetailRequestContext` с `-race -count=1` — PASS на отдельной БД gomate_test: адрес/порт/протокол сессии, auth, 404 для скрытых/отсутствующих сессий, invalid ID.
- Локальный настоящий Docker (`runner-live.json`, локальный артефакт) — PASS: requests/pwntools доступны, UID65532, нет Docker socket и секретов приложения, stdout/stderr и exit7, скрипт больше128KiB без MAX_ARG_STRLEN, timeout, отмена с дочерним процессом, лимиты контейнера/вывода/конкурентности, отсутствие оставшихся контейнеров.
- Браузер с синтетическим API (`request-workbench-ui.json`, локальный артефакт) — PASS: сохранение ручных правок, копирование и отправка именно текущего кода, безопасный текстовый вывод, Run/Stop/закрытие, размер скрипта, недоступность runner, ширина375px. Исправлены потеря фокуса после блокирования кнопки и гонка старого опроса при новом запуске.
- Браузер на Kali (`request-workbench-live.json`, локальный артефакт) — PASS: исходный HTTP-запрос отправлен с рабочей машины на реальный интерфейс Kali; сессия #97152, raw-блок #925658. Затем отредактированный requests, curl и pwntools действительно выполнены контейнерами на Kali против нашего HTTP-мока. Проверены копирование, stdout/stderr, exit7, timeout2s и Stop. Шесть тестовых запусков завершены, оставшихся runner-контейнеров нет.

Frontend и образы собраны успешно. Сохраняется прежнее предупреждение Vite о размере bundle. Dockerfile теперь использует cache mount Go build cache для повторных сборок.

## Развёртывание

Работает на `http://192.168.56.103:65000`; readiness/capture/runner и идентификаторы образов (`request-workbench-deployment.json`, локальный артефакт). Слои локальных и загруженных образов совпадают. Capture работает на eth1. Discovery получил `RUNNER_IMAGE=gomate-runner:latest` и128MiB памяти вместо64MiB.

Rollback приложения: `gomate-app:before-script-runner-20261114`; предыдущий Compose сохранён на Kali как `~/gomate-test/compose.before-script-runner.yaml`. Для отката runner не требуется миграция БД.

[Краткое руководство и лимиты](../docs/REQUEST_WORKBENCH.md). Сжатый снимок (`request-workbench-live.jpg`, локальный артефакт): JPEG quality30, 1100×760.

## Подсветка и две панели

Редактор заменён на CodeMirror с подсветкой Python/Bash, номерами строк, автоотступами, undo/redo и сохранением Tab для навигации. Он загружается отдельным chunk при открытии окна (~127 KiB gzip), поддерживает обе темы. Копирование работает и через HTTP на Kali; язык подсветки меняется без потери текста и истории.

Окно расширено до ширины экрана с отступами16px (максимум1600px). Слева редактор занимает60%, справа вывод40%; прокрутка независимая. При ширине меньше768px панели складываются вертикально. На1100×760 область кода составляет611×438px, панели высотой514px; запуск остаётся видимым.

Обновлённые браузерные проверки PASS: подсветка, темы, ширины800/1100/1440px и375px, undo, выход Tab, вставка4000 строк через paste и виртуализация, точное копирование и выполнение правок. На Kali повторно проверены все6 запусков (requests/curl/pwntools, exit7, timeout, Stop), ошибок JavaScript нет. Сборка frontend и Docker PASS; readiness/capture/runner PASS. Скриншот остался JPEG quality30,1100×760 (~51KiB).

Точка отката перед этим обновлением: `gomate-app:before-syntax-editor-20261114`.
