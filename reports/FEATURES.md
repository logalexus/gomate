# Функциональное обновление gomate

14 ноября 2026. Исходный Packmate: backend `938031f1dedfbedabef5b049368ed5b6ba6a900a`. Изменения находятся в рабочем дереве; commit/push не выполнялись. SHA-256 выбранного набора исходников: `d2d092b79451d2f929f71a5472a9bf73a998b1265c47a0e35068ee8ab55fc7e9`.

## Что добавлено

| Область | Реализованное поведение |
| --- | --- |
| HTTP | Инкрементальный HTTP/1.1, pipelining, Content-Length/chunked, gzip, URL/form decoding, user-agent hash |
| HTTP/2 и gRPC | HPACK по направлениям, HEADERS/CONTINUATION, мультиплексирование по stream ID, тела HTTP/2, gRPC envelopes, unary/streaming, gzip, RPC path и trailers |
| WebSocket | Upgrade, mask, continuation, control frames; отдельные декодированные сообщения |
| Поиск | SUBSTRING/SUBBYTES через границы raw-блоков и периодических публикаций; regex с окном 64 KiB; раздельные направления, сброс после gap |
| История | Lookback по raw/decoded, persistent checkpoint с хвостами поиска в атомарной транзакции, продолжение после перезапуска, прогресс и отмена |
| Перенос БД | Read-only источник Java, предварительная проверка настроек, импорт истории/избранного/совпадений, исходные ID, возобновляемые порции без дублей |
| Capture | Синхронный динамический cBPF по портам; повтор tuple с новым SYN создаёт новую сессию; общий бюджет активного payload |
| Потери | Независимая ограниченная очередь метаданных потерь writer; после восстановления БД затронутые строки получают incomplete; FILE не сообщает успешный commit при потерях writer |
| UI | Виртуализация списка, raw/decoded selector, metadata RPC, настройки декодеров, lookback, признаки неполноты/ошибок/вытеснений, hex-подсветка байтовых шаблонов |
| Диагностика | `/api/v2/diagnostics`: прогресс, heap/GC/goroutines, PG pool, размер БД; отдельные analysis/decode counters |

Исходные TCP-байты не заменяются результатом декодирования. Пределы и семантика описаны в [PROTOCOLS.md](../docs/PROTOCOLS.md), перенос — в [MIGRATION.md](../docs/MIGRATION.md).

## Проверки корректности

- `python3 scripts/test-integration.py`: `go test -race ./...` с отдельной БД `gomate_test` — успешно.
- `go vet ./...` — успешно. Docker frontend/Go/importer build — успешно; Vite сообщает о размере основного bundle и комментарии BootstrapVue, ошибок сборки нет.
- Новый corpus проверяет разбиение протокола до одного байта, два перемежающихся HTTP/2 stream, HPACK CONTINUATION, сжатые gRPC-сообщения, chunked/gzip/URL, WebSocket fragmentation, gzip bomb limit, malformed input, перенос флага между публикациями, gap, budget eviction, tuple reuse, общий active budget, BPF и метаданные writer loss.
- Интеграционные тесты проверяют checkpoint lookback ровно посередине флага, продолжение без состояния в памяти, отмену, metadata/empty decoded payload, representation pagination и повторный импорт legacy fixture без дублей.
- Fuzz `FuzzProtocolDecoder`, 10 секунд / 2 workers: 12 067 исполнений, падений не обнаружено. Это короткая проверка, не исчерпывающий аудит протоколов.

## Реальный сетевой путь и функциональная проверка

Машина генератора `192.168.56.102` → Kali `192.168.56.103`, capture `eth1`. Моки слушают `0.0.0.0`: HTTP/WebSocket `18080`, TCP `19090`, gRPC `15051`. SSH используется для управления и проверки БД, тестовый трафик по туннелю не идёт.

`verify-features.py` на финальной сборке:

- 10 HTTP/gzip, 10 WebSocket, 10 gRPC/gzip и 10 streaming RPC с 3 ответами каждый.
- Все 60 ответов найдены в **декодированных** блоках; проверены gzip flags, RPC path и API-представление.
- Шаблон создан после трафика: lookback нашёл все 60 декодированных ответов. Проверено 397 сохранённых блоков (raw и decoded), job завершён.
- Delta capture/queue/flow/write drops, decode errors, gaps и analysis evictions: 0.
- Машиночитаемый результат (`kali-features.json`, локальный артефакт).

Браузерный тест открывает 20 контекстов, проверяет ограниченное число отрисованных строк, decoded gRPC, text/hex, восстановление snapshot после offline/online и SUBBYTES-подсветку на новом LIVE-потоке. Результат: kali-browser-features.json (`kali-browser-features.json`, локальный артефакт), изображение: kali-features-browser.png (`kali-features-browser.png`, локальный артефакт). Это функциональная проверка; p95 реакции UI и долгий JS heap soak не измерялись.

## Повтор нагрузки

| Профиль | Запланировано | Получено клиентами / сохранено | Результат |
| --- | --- | --- | --- |
| 45 unary requests/s, 60 секунд (15 на HTTP/TCP/gRPC) | 2700 + 10 streaming RPC | Все 2700 + 30 streaming messages; errors/skips/drops/decode errors = 0 | PASS |
| 300 unary requests/s, около 10 секунд (100 на протокол) | 3000 + 10 streaming RPC | HTTP: 623 success, 310 timeout/error, 67 not-issued; TCP: 624/303/73; gRPC: 997/0/3; streaming 30 messages | FAIL |

В успешном минутном прогоне p95 времени ответа клиента: HTTP 33,299 ms, TCP 30,561 ms, gRPC 20,098 ms. Streaming RPC намеренно длится примерно 1,5 s. Это задержки сервисов/клиента, не SLO ingest-to-UI. Сохранены все ожидаемые уникальные маркеры. Результат 45 RPS (`kali-features45_60s.json`, локальный артефакт).

В неуспешном прогоне анализатор сохранил 933 HTTP, 927 TCP, 997 gRPC и 30 streaming response markers. Часть ответов существует в capture, хотя клиент сообщил timeout; эти числа нельзя выдавать за полностью обслуженную нагрузку. Счётчики drops, decode errors и DB errors = 0, но весь тест **не пройден**. Причина задержек/пропусков генератора этим опытом не локализована. Результат 300 RPS (`kali-features300.json`, локальный артефакт). Он получен на предыдущей сборке этого функционального обновления; точные image IDs записаны в JSON, последующая сборка добавила обработку пустого decoded payload и hex-подсветку.

## Среда и оставшаяся приёмка

Kali VM фактически имеет 12 vCPU / 25 061 769 216 bytes RAM, поэтому это не точное воспроизведение целевой машины 8 CPU / 16 GiB. Лимиты контейнеров анализатора сохраняются: Go 1 CPU / 2 GiB и PostgreSQL 1 CPU / 2 GiB; каждый мок 1 CPU / 128 MiB. Диагностический моментный снимок во время минутного прогона: приложение около 31 MiB, PG около 96 MiB; это не измерение пика RSS. После последнего обновления restart count приложения — 0.

Манифест сборки и лимитов (`features-build.json`, локальный артефакт). ID финального образа на Kali: `sha256:315d943a2a05d765dbb82cebc4801c18fc5f3ea6827f70b1786448df1defc27d`, mock: `sha256:d1fc44d29303e0e6c73968092b5d5c463dfd31fc1678d231756f2b5515d229ec`. При переносе через docker save/load локальные и Kali image IDs отличаются; filesystem layer digests и Created совпадают, различаются отсутствующие/пустые поля конфигурации Docker. Сравнение (`feature-image-comparison.json`, локальный артефакт).

TLS decryption, protobuf schema field names, WebSocket extensions, h2c upgrade/server push и IP defragmentation не реализованы. Эти ограничения перечислены в документации и не скрываются за raw-сохранением. Native WebSocket заменил legacy SockJS, frontend обновлён вместе с API.

Шестичасовой прогон, 200 независимых источников с checker-профилем, matched baseline без анализатора, сравнение Java/Go и окончательное подтверждение SLO/влияния на сервисы остаются незавершённой приёмкой. **Функциональное обновление не означает завершения всего PLAN.md или готовности к высокой соревновательной нагрузке.** Стенд оставлен работающим для продолжения проверок.
