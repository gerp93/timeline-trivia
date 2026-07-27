# CLAUDE.md — timeline-trivia

Guidance for working in this repository. This file is a **style guide first,
an architecture map second**. It documents the conventions already in use so
that changes match the existing codebase. Match the surrounding code; do not
introduce new styles, formatters, or abstractions.

This repo shares its platform and style with
[card-judge](https://github.com/gerp93/card-judge) — both consume
[`gameshell-framework`](https://github.com/gerp93/gameshell-framework) and
read as one author's codebase. When in doubt, check how card-judge does the
same thing.

## What this is

**Timeline Trivia**: players are dealt event cards one at a time and must
place each into the correct chronological position in their own timeline. A
correct placement grows the timeline; an incorrect one discards the card.
First player to reach the configured number of cards wins. Lobbies can
optionally restrict the draw pile to one or more year ranges.

Stack: **Go (stdlib `net/http`) + HTMX + `gorilla/websocket` + MariaDB.** No
web framework, no ORM, no build step for the front end.

## Layout

The repo root is a thin wrapper. **All application code lives under `src/`,
which is the Go module root** (`module github.com/gerp93/timeline-trivia`, Go
1.22.5). The reusable platform lives in the separate
**`github.com/gerp93/gameshell-framework`** module (auth, page middleware,
user/lobby-shell/player-base data layer, **deck management**, shared chat
rendering, websocket hub, framework schema) — this repo holds only the game.

```
src/
  main.go                entry point: registers the Game impl + framework
                         params, DB connect, framework schema then game
                         schema, ALL route wiring, server
  go.mod                 module + framework dependency (pinned version tag)
  game/                  hooks.go — TimelineTrivia implements gameshell.Game
  api/                   game HTTP handlers, grouped by domain
    pages/                full-page renderers (package apiPages)
    user/ access/         packages apiUser, apiAccess
    card/                 card CRUD + CSV export (package apiCard)
    timelinetrivia/       gameplay handlers (package apiTimelineTrivia)
  database/               game data-access: one file per domain
    card.go                card CRUD, CSV export, deck-delete audit hook
    timeline-trivia.go     game/draw-pile/timeline/year-range logic
  static/                 embedded assets (//go:embed)
    static.go              embed.FS + SQLFiles (ORDERED game schema
                           manifest, runs AFTER the framework schema)
    sql/                   game tables/triggers under src/static/sql/
    html/                  pages/body/* (this game's own pages — NOT
                           base.html, login/users/decks/deck-access/account,
                           which are framework-owned, see below) and
                           components/ (HTMX fragments)
    css/ js/ images/
tests/                    setup + theme-validator tooling (own go.mod each)
```

There is intentionally **no `cmd/`, `internal/`, or `pkg/`** — flat top-level
packages under `src/`. Keep it that way. Handlers that need framework data
functions import them as `gsDatabase "github.com/gerp93/gameshell-framework/database"`,
`gsApi "github.com/gerp93/gameshell-framework/api"`, etc., alongside the game
`database`/`api` packages.

## The most important architectural fact

Unlike card-judge, **game logic here lives in Go, not SQL**. The SQL schema
(`src/static/sql/`) is just tables + a couple of housekeeping triggers
(changed-on-date, card-delete/update audit) — there are no `SP_*`/`FN_*`/`V_*`
game-rule objects. Draw-pile initialization, year-range filtering, turn
advancement, and win detection are all plain Go functions in
`database/timeline-trivia.go`, called from `api/timelinetrivia`. When you
change game behavior here, you are almost always editing Go.

Schema is applied by iterating `static.SQLFiles` (in `src/static/static.go`)
on every server start via `gsDatabase.RunFile`/`gsDatabase.Execute`, **after**
the framework's own `gsStatic.SQLFiles` have run (game `CARD` FKs to the
framework's `DECK`). Order matters and is manual — tables in dependency order,
then triggers.

## Deck / card split (framework owns decks, game owns cards)

- **Decks are framework-owned**: `DECK`, `USER_ACCESS_DECK`, `AUDIT_DECK`,
  deck triggers, and the `api/deck` CRUD handlers all live in
  `gameshell-framework` and are mounted directly in `main.go`
  (`gsApiDeck.Create`, `.SetName`, `.SetPassword`, `.SetIsPublicReadOnly`,
  `.Delete`). This repo does not duplicate any of that.
- **Cards are game-owned**: `CARD(ID, CREATED_ON_DATE, CHANGED_ON_DATE,
  DECK_ID FK→DECK ON DELETE CASCADE, TEXT, CARD_YEAR INT NULL)` +
  `AUDIT_CARD`, with CRUD in `database/card.go` and handlers in `api/card`.
  `CARD_YEAR` is **authored data** entered when the card is created/edited —
  there is no text-scraping/regex year parsing; a card with a NULL year is
  simply excluded from the draw pile.
- **`OnDeckDeleting` hook** (`game/hooks.go`): MariaDB's `ON DELETE CASCADE`
  from `DECK` to `CARD` does **not** fire `CARD`'s own triggers, so the
  framework calls this hook before deleting a `DECK` and the game audits its
  own cards (`database.AuditDeckCardsAsDeleted`) in response. If you add more
  game-owned tables that FK to `DECK`, extend this hook, not a trigger on the
  framework's `DECK` table.
- **The deck detail page (`/deck/{deckId}`) follows the same split**: the
  chrome (header, Export Deck, the Edit Deck dialog, the danger-zone delete)
  is `gameshell-framework`'s `deck-detail-chrome.html`; the card table and
  create/edit-card dialogs — genuinely game-specific, since `CARD_YEAR` +
  category and the Import Cards feature don't exist in every game — are
  this repo's own `static/html/pages/body/deck-card-management.html` and
  `deck-search-controls.html`, composed with the chrome via
  `gsApiPages.ParseGameFragment` in `api/pages/pages.go`'s `Deck` handler.
  See the **body-name collision rule** below before touching either side.

## Pages owned by the framework, not this repo

Login, admin user management (`/users`), the deck list (`/decks`), the deck
password gate (`/deck/{deckId}/access`), `base.html`, and the account page's
shared chrome (theme picker, name, password, danger-zone) are **not**
rendered by this repo — they're byte-identical (or were, until reconciled)
to card-judge's own copies, so `gameshell-framework`'s `api/pages` package
(`gsApiPages`) owns the template *and* the `http.HandlerFunc`; `main.go`
mounts them directly (`gsApiPages.Login`, `.Users`, `.Decks`, `.DeckAccess`,
`.Account`) the same zero-wrapper way `gsApiDeck`'s CRUD handlers are
mounted. Every page handler this repo still owns parses the framework's
`base.html` via `gsStatic.StaticFiles`, never a local copy — there isn't
one anymore.

The account page's win-celebration section is opt-in, not automatic:
`main.go` calls `gsApiPages.SetAccountPageFeatures(gsApiPages.AccountPageFeatures{WinCelebration: true})`
at startup. Don't remove that call without also removing the
`SetWinGif`/`ClearWinGif`/`GetWinGif`/`SetWinMessage` route mounts — the
section would otherwise render pointing at routes that don't exist.

**Body-name collision rule**: every page body template in this repo and the
framework defines the same Go template name, `{{define "body"}}` — this
only works because exactly one body file is ever parsed per request
(`parseChrome` in `api/pages/pages.go` enforces this: one `ParseFS` call
against the framework's `base.html`, one against this repo's own body
file). A composed parse (like `Deck`'s) must never include two files that
both define `"body"` — `text/template` silently lets the second overwrite
the first, with no compile-time signal. `deck-card-management.html` and
`deck-search-controls.html` define distinctly-named blocks
(`card-header-actions`, `card-management`, `card-search-controls`) for
exactly this reason — never rename either to `"body"`.

## Year-range filtering

`TIMELINE_TRIVIA_YEAR_RANGE(ID, TIMELINE_TRIVIA_GAME_ID, FROM_YEAR, TO_YEAR)`
holds zero or more inclusive `[FromYear, ToYear]` filters per game (empty =
all years allowed). `database.GetYearRanges`/`AddYearRange`/
`ApplyYearRangeFilter` in `database/timeline-trivia.go` manage them; the draw
pile is filtered to cards whose `CARD_YEAR` falls in at least one range. The
lobby header (`static/html/pages/body/timeline-trivia.html`) renders one pill
chip per active range — keep that in sync if the range shape changes.

## Go conventions (match these exactly)

- **Package naming:** subpackages under `api/` are named `api<Thing>` even
  though the directory is lowercase — package `apiCard` in `api/card/`,
  `apiTimelineTrivia` in `api/timelinetrivia/`, `apiPages` in `api/pages/`.
  Top-level packages (`database`, `game`, `static`) match their directory.
  `gofmt`/tabs.
- **Handlers** have the shape `func Name(w http.ResponseWriter, r *http.Request)`
  and are wired in `main.go` with Go 1.22 method+pattern routes
  (`http.Handle("POST /api/...", gsApi.MiddlewareForAPIs(http.HandlerFunc(...)))`).
- **Form/param parsing** uses the range-switch idiom, not a decode library:
  ```go
  for key, val := range r.Form {
      switch key {
      case "text":
          text = val[0]
      }
  }
  ```
- **Responses are plain text**, written directly — no JSON envelope:
  ```go
  w.WriteHeader(http.StatusBadRequest)
  _, _ = w.Write([]byte("No card found."))
  ```
  Messages are human-readable sentences, capitalized, ending with a period.
  The `_, _ =` discard on `Write` is deliberate and consistent — keep it.
- **DB layer:** raw SQL strings passed to `gsDatabase.Query`/`gsDatabase.Execute`
  (or the game's own `database` package wrapping them). Multi-line SQL uses
  backtick literals; one-liners use double quotes. Read results row-by-row
  with `defer rows.Close()` then `rows.Scan(...)`. On scan error the pattern
  is `log.Println(err); return ..., errors.New("failed to scan row in query results")`.
  Structs mirror table columns (PascalCase fields, `sql.Null*` for nullables,
  e.g. `Card.Year sql.NullInt64`). No ORM, no query builder — do not
  introduce one.
- **IDs** are `uuid.UUID` (`github.com/google/uuid`), generated with
  `uuid.NewUUID()` in Go or `UUID()` in SQL.
- **Config** is environment variables via `os.Getenv`, all prefixed
  `TIMELINE_TRIVIA_` (`_SQL_HOST/_SQL_DATABASE/_SQL_USER/_SQL_PASSWORD`,
  `_PORT`, `_LOG_FILE`, `_CERT_FILE`, `_KEY_FILE`). No config files or
  libraries.

## SQL conventions (match these exactly)

- **Uppercase everything** — keywords AND identifiers (table/column names).
- **One database object per file**, named after the object, using prefixes:
  `TR_` trigger, `AUDIT_` history table. (No `SP_`/`FN_`/`V_` objects exist
  in this repo today — see "most important architectural fact" above.)
- Tables use `CREATE TABLE IF NOT EXISTS`; triggers use `CREATE OR REPLACE`
  so re-running the manifest is idempotent.
- **Format with the repo's formatter**, not by hand:
  `src/static/sql/sqlfmt.sh` runs `sqlfmt --newlines --upper --spaces 4
  --comment-pre-space` over every `*.sql`. Run it after editing SQL.
- After adding/removing a SQL file, update `SQLFiles` in `src/static/static.go`.

## Real-time (websocket) pattern

Messages over the socket are **short control strings, not structured
payloads**, except `result:`, whose payload is JSON (below). Control strings:
`refresh`, `reload`, `result:<json>`, `status:<text>`, `chat:...`, `alert:...`,
`lobbyMessage:<text>`, `turnTimer:<seconds>`, `kick`. The server broadcasts a
hint and the browser (`src/static/js/timeline-trivia.js`) reacts by
re-fetching the relevant HTML fragment via `htmx.ajax`/`fetch` from
`/api/timeline-trivia/{lobbyId}/...` routes. HTML is never pushed over the
socket. Chat message rendering (color tokens, timestamp, history trim) is
**shared with card-judge** via `gameshell-framework`'s `static/js/chat.js`
(`window.gsChat`), mounted at `/gs/js/chat.js` — do not reintroduce a local
copy.

**The bottom-of-screen status line and result popup are driven only by the
websocket, never by an HTTP response.** `result:` carries a `bottomMessage`
that every client (including whoever acted) writes into
`#timeline-trivia-message`; `status:` is the same idea without a popup, for
non-guess events like Skip & Remove. The place-card/skip-card buttons
(`timeline.html`, `current-card.html`) use `hx-swap="none"` specifically so
htmx never swaps the HTTP response into that div — a `hx-target`/`hx-swap`
pointed at `#timeline-trivia-message` previously let the acting player's own
response race the websocket broadcast and overwrite it, so only that one
player saw different text than everyone else. Don't reintroduce an
`hx-target` on those buttons; if a handler needs to tell the *acting* browser
something no one else should see, that's what the plain-text HTTP response
body is still for (it's just not rendered anywhere by default).

`result:`'s `nextPlayerName` field (omitted when the game just ended) is who
the client should run a "3, 2, 1, `<name>`'s turn" countdown for once the
result popup clears, before the real per-turn timer starts — see
`showTurnCountdown`/`deferTimerStart` in `timeline-trivia.js`. The timer must
never visibly start (or keep ticking) behind a popup; `deferTimerStart` gates
`restartTurnTimer` for exactly that window.

Note the `reload` case specifically waits ~500ms before refreshing rather
than doing a full page navigation: a `location.reload()` drops the websocket
connection, and if this player is the only client, the framework deletes the
(now-empty) lobby before the reload finishes, destroying the game that was
just started/reset.

## HTML conventions

**Navigation is always a real `<a href>`.** Anything that takes the user to
another page must be wrapped in an anchor — never
`onclick="location.href='...'"`, and never a JavaScript click-interceptor.
Only a real anchor gets ctrl/cmd-click and middle-click to open a new tab,
right-click → "Open in new tab", the hover URL preview, and link semantics for
screen readers; a script cannot reproduce all of that, and this is the same
convention card-judge follows.

```html
<a href="/decks"><button>Card Decks</button></a>
<a href="/timeline-trivia/{{.Id}}"><button class="btn-small">Join</button></a>
<a href="/account" class="no-style">
    <div class="top-bar-menu-link">Account <i class="bi bi-gear"></i></div>
</a>
```

`a.no-style` (`static/css/global.css`) strips the anchor's underline/color so
a wrapped menu row looks unchanged. Buttons that perform an *action* rather
than navigate (`hx-post`/`hx-put`/`hx-delete`, opening a `<dialog>`) stay
plain buttons. External links additionally take `target="_blank"`.

## Build / run / verify

- Build: `cd src && go build ./...`.
- Run: needs a MariaDB reachable via the `TIMELINE_TRIVIA_SQL_*` env vars;
  create the DB once with `src/static/sql/setup.sql`, then the server applies
  the rest of the schema (framework, then game) on startup. Serves on `:2016`
  (or `TIMELINE_TRIVIA_PORT`).
- Docker: root `Dockerfile` builds and runs the binary.
- Versioning: `version_bump.sh {major|minor|patch}` (own version, tracked
  separately from `gameshell-framework` and card-judge).
- Deployment tooling lives in the separate `gameshell-deploy` repo; this repo
  only keeps `deploy.conf` + `backups/`.
- `tests/` holds setup helpers and a standalone theme-validator, each with
  their own `go.mod` — unrelated to the game itself.
- Game-level automated coverage lives in `src/`: `e2e_test.go` drives the real
  HTTP handlers against a real database with real websocket clients (sessions
  minted via `auth.SetUserId`, valid in-process since the framework's signing
  secret is per-process) — it's the main regression net for turn order,
  steals, timeouts, chat/status-line text, and Skip & Remove. It refuses to
  run unless `TIMELINE_TRIVIA_SQL_DATABASE` starts with `tt_e2e`, since it
  seeds and mutates freely: `TIMELINE_TRIVIA_SQL_DATABASE=tt_e2e go test ./...`
  against a throwaway database (create it first; the schema runner creates
  tables but not the database itself). `win_celebration_test.go` (same
  `tt_e2e` requirement) covers the account-page GIF/PNG upload and message
  length limit. `pages_render_test.go` (same requirement) renders the
  framework-owned pages plus `Deck`'s chrome+fragment composition and
  asserts on real content — these handlers discard `ExecuteTemplate`'s
  error, so a template/data mismatch fails silently (a 200 with a truncated
  body) rather than with a visible error; a status-code-only check would
  miss that. `database/timeline-trivia_test.go` covers pure-function bounds
  (e.g. `ValidateCardsToWin`) with no DB needed.
- Still **verify UI/visual changes by running the app and playing through the
  affected flow** (create a lobby, optionally with a year-range filter, join
  with two players, place cards correctly and incorrectly, confirm a win) —
  the automated tests exercise the Go handlers and JS logic, not layout,
  color contrast, or animation.

## Known quirks (preserve unless explicitly changing)

- The full SQL schema (framework, then game) re-runs on every startup
  (idempotent by design).
- The lobby is **deleted when its last websocket client disconnects**
  (framework `websocket/hub.go`).
- The auth signing secret is process-random (framework `auth/cookie.go`), so
  sessions do not survive a restart and cannot be shared across instances —
  after restarting locally you'll need to log back in.
- A card with `CARD_YEAR IS NULL` is authored-but-incomplete; it's silently
  excluded from every draw pile rather than erroring.
- `ResetTimelineTriviaGame` deliberately does **not** clear
  `TIMELINE_TRIVIA_PLAYER_ORDER`. `ShuffleTimelineTriviaPlayerOrder` (called
  from `StartTimelineTriviaGame`) needs the still-there rows as the "previous
  order" baseline so it can guarantee the new shuffle differs from the last
  game's — not just probably differ, which a fresh `rand.Shuffle` alone only
  gives you (1/N! chance of reproducing the same order by pure luck). If you
  re-add a clear-on-reset, that guarantee silently degrades back to
  probabilistic.
