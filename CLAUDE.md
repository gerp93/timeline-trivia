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
    html/                  pages/ (base.html + body/*) and components/
                           (HTMX fragments)
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
payloads** (`refresh`, `reload`, `result:...`, `chat:...`, `alert:...`,
`kick`). The server broadcasts a hint and the browser
(`src/static/js/timeline-trivia.js`) reacts by re-fetching the relevant HTML
fragment via `htmx.ajax`/`fetch` from `/api/timeline-trivia/{lobbyId}/...`
routes. HTML is never pushed over the socket. Chat message rendering
(color tokens, timestamp, history trim) is **shared with card-judge** via
`gameshell-framework`'s `static/js/chat.js` (`window.gsChat`), mounted at
`/gs/js/chat.js` — do not reintroduce a local copy.

Note the `reload` case specifically waits ~500ms before refreshing rather
than doing a full page navigation: a `location.reload()` drops the websocket
connection, and if this player is the only client, the framework deletes the
(now-empty) lobby before the reload finishes, destroying the game that was
just started/reset.

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
- There is no automated test suite for the game itself (`tests/` holds setup
  helpers and a standalone theme-validator, each with their own `go.mod`);
  **verify game changes by running the app and playing through the affected
  flow** (create a lobby, optionally with a year-range filter, join with two
  players, place cards correctly and incorrectly, confirm a win).

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
