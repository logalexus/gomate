# Traffic search

The compact search field above the service selector searches saved traffic. Enter
text and press **Enter** or the search button. Enable **`.*`** for a Go RE2 regular
expression, for example `token=[a-z0-9]+` or `/api/(login|register)`. Both modes
ignore case by default; regex supports `(?-i)` to require exact case. Text mode
treats punctuation literally. Invalid expressions appear below the field.

Search combines with the selected service, Favorites, and **all** selected pattern
labels. The query and mode survive session navigation, refresh, and browser Back.
Clear search with its × button or Escape; Reset all filters also clears it.

Search covers each saved raw or decoded packet's content, plus first request
method/path, client/server addresses, and protocol. Packet matches return the
containing session once. It does not join separate packets for matching and does
not automatically decode arbitrary Base64 blobs. Retained request metadata remains
searchable after packet cleanup, while removed payload cannot be searched.

Search runs explicitly over history. It does not repeat on LIVE refresh events.
The header changes to Search and shows progress; stop with the square button,
then Continue from the last checkpoint, or restart with the header/search button.
Older/Newer paginate matching sessions; Latest starts again at newest traffic.
Capture continues while searches run.

`POST /api/v2/traffic/search` accepts `query`, `mode` (`text`/`regex`), `port` (0 for
all), `patternIds`, `favorites`, `pageSize` (1–100), and the opaque `cursor` returned
by the previous request. Response: `{streams, next, scanned}`. An empty `streams`
array with a non-null `next` is an intermediate batch, not the end of the search.
The UI follows these checkpoints automatically until a results page is full or
history ends. History can change during a search due to capture/retention.

Each request considers at most 128 candidate sessions and yields after 512
packets, about 8 MiB of payload, or 500 ms, checked between packets/sessions. A
single packet or database operation may exceed these soft byte/time budgets;
the normal request deadline still applies. Continuations resume inside large
sessions rather than dropping the remainder. At most two search requests run at
once; additional requests receive 429 and Retry-After. This reserves database pool
capacity for capture and other UI operations. Regex execution uses Go's bounded
RE2 engine, never a user expression inside a database query. No schema migration
or payload duplication is required.

## Verification

- `go test ./...` and race checks for API/model/storage.
- PostgreSQL integration tests cover literal/regex and binary content, combined
  filters, history beyond the candidate window, packet and byte continuations,
  validation, and metadata after payload cleanup.
- `node --test tests/browser/*.test.mjs` checks route-state preservation and
  other frontend helpers.
- `node tests/browser/traffic-search.cjs` checks forms, cancellation, pagination,
  validation, and responsive layouts against a mocked API.
- `node tests/browser/traffic-search-live.cjs` reads actual traffic on Kali and
  checks both search modes and capture continuity. It requires `.cache/kali.env`.

Browser and deployment results are saved in `reports/traffic-search-*.json`.
