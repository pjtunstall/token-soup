# Chat persistence (`DATABASE_URL` and `POST /api/chat`)

## Table of contents

- [Overview](#overview)
- [Happy path](#happy-path)
  - [Behavior](#behavior)
  - [Implementation](#implementation)
- [Sad path](#sad-path)
  - [Failure modes (quick links)](#failure-modes-quick-links)
  - [Persistence disabled (no `DATABASE_URL`)](#persistence-disabled-no-database_url)
  - [First turn: create chat + first user message fails](#first-turn-create-chat--first-user-message-fails)
  - [Existing chat: append user message fails](#existing-chat-append-user-message-fails)
  - [Insert assistant (bot) message fails](#insert-assistant-bot-message-fails)
  - [Update chat title fails](#update-chat-title-fails)

---

## Overview

The Go API can optionally persist chats and messages to Postgres when `DATABASE_URL` is set at startup. The browser still talks to Supabase only for **auth**; it talks to the Go API for chat. Persistence is **best-effort**: the API prefers clear outcomes (stream + warnings, or HTTP errors before streaming) over silent divergence.

OpenRouter **turns** are built from an in-process **`HistoryStore`**, keyed by user and (when applicable) chat id. The database is the **durability** layer for messages, not the source of truth for model context during a normal request (reload-from-DB for cold starts is a future concern).

---

## Happy path

### Behavior

1. User signs in; the web app holds a Supabase **JWT** and calls **`POST /api/chat`** with `Authorization: Bearer …`, JSON `message`, `senderName`, and optionally `chatId` / `ephemeral` (see sad path).
2. **First message** of a persisted thread (no `chatId` yet, not in ephemeral mode): the server creates a **`chats`** row and the first **`messages`** row (user) in one transaction, then sends an SSE event **`chat_session`** with **`chatId`**. The client stores that id for later requests.
3. **Later messages**: the client sends the same **`chatId`**. The server inserts the user row, appends to in-memory history for that thread, streams the assistant via OpenRouter, then inserts the assistant row and appends the assistant to memory.
4. On the **first** user message of a thread, a background title is generated; the UI receives **`chat_title`** on SSE, and the server **`UPDATE`s `chats.title`** when persistence is active for that thread.

### Implementation

- **Startup** (`cmd/token-soup/main.go`): if `DATABASE_URL` is non-empty, a `pgxpool.Pool` is created and wrapped in `chat.Persist`; otherwise `persist` is nil and a warning is logged.
- **Handler** (`internal/chat/handler.go`): JWT subject → `user_id`; body decoded into `message`, `senderName`, `chatId`, `ephemeral`.
- **Persistence** (`internal/chat/persist.go`): SQL insert/update helpers; each uses **`withPersistRetry`** (`internal/chat/retrydb.go`).
- **Memory** (`internal/chat/store.go`): `AppendUser` / `AppendUserChat` / `AppendBot` / `AppendBotChat` keyed by `userID` or `userID + chatId`.
- **SSE** (`internal/openrouter/types.go`, handler): events include `chat_session` (`chatId`), `chat_title`, `persist_warning`, plus existing `ai_*` stream events.
- **Web** (`web/src/lib/chat.ts`, `web/src/routes/+page.svelte`): sends `chatId` and `ephemeral` when applicable; shows `persistWarning` banner on `persist_warning`; clears `chatId` / ephemeral flags on sign-out.

---

## Sad path

Operations in `internal/chat/persist.go` run through **`withPersistRetry`** (`internal/chat/retrydb.go`): up to **five** attempts, exponential backoff capped at **two seconds**, only for errors classified as **retryable** (e.g. timeouts, connection drops, selected Postgres connection error codes, common “connection refused”-style messages). **Non-retryable** errors return after the first failed attempt. **`context.Canceled`** is not retried.

What the user sees depends on **which** operation failed and whether the client already has a **`chatId`**.

### Failure modes (quick links)

- [Persistence disabled (no `DATABASE_URL`)](#persistence-disabled-no-database_url)
- [First turn: create chat + first user message fails](#first-turn-create-chat--first-user-message-fails)
- [Existing chat: append user message fails](#existing-chat-append-user-message-fails)
- [Insert assistant (bot) message fails](#insert-assistant-bot-message-fails)
- [Update chat title fails](#update-chat-title-fails)

### Persistence disabled (no `DATABASE_URL`)

#### Failure

Persistence is off: nothing is written to Postgres. Chat still works for the life of the process using a **single in-memory thread per user** (no `chatId` from the server).

#### Remedy

**Implemented today:** The API starts without a pool when `DATABASE_URL` is missing or empty; it logs a warning and serves chat using **in-memory history only** (no Postgres). No UI change is required—the app keeps working until the process exits.

**Further actions or future refinements:** To enable persistence, an operator must set `DATABASE_URL` and **restart** the API. There is no in-app configuration path. A future refinement could surface “persistence off” in the UI or block chat until configured, if product requirements call for it.

#### Implementation

`persist` is nil; handler uses only `HistoryStore.AppendUser` / `AppendBot` with the legacy per-user key. No `chat_session` or DB-backed title update.

### First turn: create chat + first user message fails

#### Failure

After retries, the transaction that creates **`chats`** plus the first user **`messages`** row still fails. The user still gets a **normal streamed reply** for that request, but **no** `chat_session` event—so the client **never receives a `chatId`**. A **`persist_warning`** SSE explains that the conversation is only in memory. The title may still appear in the UI via **`chat_title`**, but there is **no** `chats` row to update.

#### Remedy

**Implemented today:** The server still completes the request: it streams the assistant reply from **in-memory** history (`AppendUser` with the legacy per-user key), emits **`persist_warning`** on SSE, and does **not** emit **`chat_session`**. The web client sets **`persistEphemeral`** when it sees that warning while **`chatId`** is still null, and on later sends includes **`ephemeral: true`** (and no **`chatId`**) so the handler takes the ephemeral branch and **does not** retry **`CreateChatWithFirstUserMessage`** on every message—avoiding a pile of empty or half-created **`chats`** rows if the DB recovers mid-conversation.

**Further actions or future refinements:** Operators must **restore database connectivity** (or credentials) so that a **new** “first message” without **`ephemeral`** can create a row successfully; there is no automatic backfill of the in-memory thread into Postgres. Possible refinements: a visible “retry saving” control, **outbox/retry queue** for failed creates, or **reconciliation** once the DB is healthy again.

#### Implementation

On create error: `warnPersistFailed` → SSE `persist_warning`; fallback to `AppendUser` (legacy key); `dbChatID` stays `0`; `saveBotToDB` false; assistant stored with `AppendBot` only. Title goroutine skips `UpdateChatTitle` because `saveBotToDB` is false.

### Existing chat: append user message fails

#### Failure

The client sent a **`chatId`** from a prior **`chat_session`**, but inserting the **new user** row fails after retries. The handler returns **HTTP 503** **before** any SSE body. There is **no** assistant reply for that request.

Because the web app **optimistically** adds the user bubble **before** `fetch` completes, the user may **see** their line in the UI even though the server did not persist it and did not add it to server memory for that chat. Retrying by sending again can **duplicate** user bubbles unless the UI is improved later.

#### Remedy

**Implemented today:** The request ends with **HTTP 503** and a plain error body; **`postChat`** throws, and the client’s **`catch`** path **`pushAssistantBubble`** with the error text—so the user sees an assistant-styled **error line**, not a model reply. There is no SSE stream for that attempt. The optimistic **user** bubble remains on screen (we do not roll it back).

**Further actions or future refinements:** The user (or operator) must **fix** the underlying DB issue (connectivity, RLS if ever enabled, schema, disk, etc.); **retrying** the same send is manual and may add a **second user bubble** with the same text. Future refinements: **remove or mark the failed user bubble** on 503, **single-message retry** without duplicating, or **automatic retry** with idempotency keys on the server.

#### Implementation

`persist.AppendUserMessage` error → `log.Printf` → `http.Error(..., 503)` → `return` before `WriteHeader` / SSE. No `AppendUserChat` for that message on success path—so server memory for that chat does not include the failed line.

### Insert assistant (bot) message fails

#### Failure

The user row was already committed; the stream finished; **`AppendBotChat`** has already updated server memory. **`InsertBotMessage`** then fails after retries. The user already saw the full assistant text; they receive **`persist_warning`** explaining the assistant row was not saved.

#### Remedy

**Implemented today:** **`AppendBotChat`** runs **before** **`InsertBotMessage`**, so the in-process **`HistoryStore`** already includes the assistant turn; later messages in **this** server process still see full context. The client receives **`persist_warning`** after the stream with copy explaining the assistant row was not stored. No automatic retry of the insert is attempted after the handler’s **`withPersistRetry`** completes.

**Further actions or future refinements:** Operators may fix DB issues and users can **keep chatting**; the DB will be **missing** that assistant row until a **manual backfill**, **reconciliation job**, or **re-sync from client history** exists. Possible refinements: **dead-letter queue** for failed inserts, **background retry**, or **read-your-writes** check before closing the stream.

#### Implementation

Order in handler: `AppendBotChat` then `InsertBotMessage`. Warning emitted via `persist_warning` if insert returns error.

### Update chat title fails

#### Failure

The title string is still sent to the client as **`chat_title`** on SSE. The **`chats.title`** column may remain at its default if the **`UPDATE`** fails.

#### Remedy

**Implemented today:** The **`chat_title`** SSE event is still sent to the client, so the **subtitle in the UI** updates. Failure to **`UPDATE`** **`chats.title`** is only **`log.Printf`** on the server; the user is **not** notified of the DB mismatch.

**Further actions or future refinements:** Operators can inspect logs and Postgres; **`chats.title`** can be edited manually or by a **one-off script**. Possible refinements: **retry** the update in the goroutine, emit a **non-fatal warning** SSE, or **re-run title generation** when loading a chat from the DB later.

#### Implementation

`UpdateChatTitle` errors are **`log.Printf` only** inside the title goroutine; SSE title event is still sent.
