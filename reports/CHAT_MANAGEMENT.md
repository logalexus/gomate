# Shared message editing and deletion

Every participant with access to gomate can edit or delete any chat message, including messages sent from another browser. Use the message's ellipsis menu. Edits preserve Markdown, the original author and creation time; edited messages show a timestamped marker. Save/Cancel and Ctrl/Cmd+Enter/Escape are supported. Deletion requires an in-app confirmation.

PATCH `/api/v2/chat/:id` accepts `{text, revision}`; a stale revision returns 409 instead of overwriting another edit. DELETE returns an idempotent deletion marker. Existing authentication, origin validation and shared chat rate limits apply to both operations.

Schema version 11 adds a revision sequence, revision index, edit timestamp and deletion marker. Mutations use the existing chat transaction lock to order revisions with commits. Deleted message bodies are overwritten, and tombstones follow the existing seven-day/10,000-row retention policy. GET chat exposes a bounded change feed using `revision`, including `changesOnly=1` while browsing history. Clients retain at most 500 displayed messages and a bounded cache of recent message revisions to reject stale snapshots.

Validation completed on 2026-11-15:

- `.tools/go/bin/go test ./...` passed.
- `.tools/go/bin/go test -race ./internal/api ./internal/storage` passed.
- All 26 PostgreSQL integration tests passed using an isolated local test database. Added coverage for foreign-message edits/deletes, conflict rejection, concurrent writers, missing/invalid revisions, change pagination, updates outside the latest history page, deletion retries, content clearing, send retries after deletion, authentication and origin checks.
- `node --test tests/browser/*.test.mjs tests/*_test.mjs` passed: 40 tests, including stale-snapshot handling, tombstones, bounded memory and history merging.
- `npm --prefix web run build` passed. Existing vendor annotation and bundle-size warnings remain.
- `node tests/browser/chat-management-live.cjs` passed against Kali using two independent browser contexts. Verified formatted foreign-message edits, synchronization, cancellation, draft preservation, stale-edit conflicts, deletion confirmation/cancellation, remote deletion of a message with an open editor, persistence after reload and 390px layout. No browser page errors; no screenshots taken. The test only edited/deleted its own fixture message.

Deployment: `gomate-app:chat-management-20261115t195311z` at http://192.168.56.103:65000. Only the app container was recreated; other containers retained their image and start time. See [deployment details](chat-management-deployment.json) and [live browser checks](chat-management-live-check.json).
