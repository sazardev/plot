# STACK.md: Go terminal data plotter (CLI + Bubble Tea TUI)

## Summary

`pl` is a CLI-first terminal data plotter in Go: it reads delimited text from stdin or files, or runs a read-only
SQLite query, infers columns and renders line, bar, pie and donut charts as terminal characters. The default path
is pipe-friendly and has no TUI dependency at runtime. `--tui` opts into a Bubble Tea view, which follows the Elm
architecture: a `Model` holds state, `Update` turns messages into a new model plus commands, `View` renders a string.
Side effects (IO, timers, cache writes) run only inside `tea.Cmd`s and come back as messages. Domain, parsing,
rendering and configuration are plain Go packages outside the UI.

## Stack

| Layer | Choice | Why |
| --- | --- | --- |
| Language | Go 1.26+ | One static binary, fast startup, trivial cross-compilation |
| CLI entry | `github.com/spf13/cobra` + `pflag` | Flags, help, usage errors and exit-code mapping |
| TUI runtime | `github.com/charmbracelet/bubbletea` (`--tui` only) | Elm architecture: predictable state, side effects as commands |
| Components | `github.com/charmbracelet/bubbles` (spinner, help, key) | Tested building blocks with key bindings and help |
| Styling | `github.com/charmbracelet/lipgloss` | Declarative styles, layout, adaptive colors |
| Renderer | `internal/render` (plain Go) | Pure chart layout, braille/block glyphs and ANSI; no Bubble Tea or Lip Gloss |
| Input | `encoding/csv` with detection + `github.com/mattn/go-runewidth` | RFC 4180 parsing and terminal cell widths |
| Database source | `modernc.org/sqlite` (pure Go, CGO-free) | Read-only local SQLite; PostgreSQL/MySQL arrive through `psql --csv`/`mysql --batch` pipes |
| Cache | SQLite + `sqlc` + `goose`-authored migrations | Typed cache queries, versioned schema, WAL, disposable data |
| Config | TOML (`github.com/BurntSushi/toml`) in `os.UserConfigDir()/pl` + env/flags | Strict keys, documented precedence: flag > env > config > default |
| Logging | `log/slog` to `DEBUG`-gated `os.UserCacheDir()/pl/debug.log` | stdout belongs to the chart; stderr to user-facing errors |
| Internationalization | go-i18n v2 (`nicksnyder/go-i18n`, TOML files embedded with `embed`) + `golang.org/x/text` | CLDR plurals, locale number separators, English fallback |
| Testing | `testing` + `stretchr/testify` + `teatest` + fuzz targets | Unit, golden, whole-program and parser/inference fuzzing |
| Lint | golangci-lint v2 | One config, many linters |
| Release | GoReleaser (`.goreleaser.yaml`): static linux/darwin/windows archives, checksums, SBOM | One tag = a complete, reproducible release |
| CI | GitHub Actions (`.github/workflows/ci.yml`) on every pull request and push to `main` | Runs the same quality gates as local |
| Hooks | Lefthook (`lefthook.yml`): pre-commit format + lint, commit-msg Conventional Commits | Problems caught before they reach CI |
| Updates | Renovate (`renovate.json`): grouped weekly updates | Dependencies stay current in small reviewable steps |
| Web playground | Chart.js (MIT), SheetJS (Apache-2.0), sql.js (MIT), jsPDF (MIT), Geist fonts (OFL), vendored in `docs/assets/vendor` | Browser-only sandbox with exports/history; never part of the Go binary |

## Project layout

```
cmd/pl/                          entry point: flags, config, source selection, CLI pipeline, TUI wiring, exit codes
internal/
  config/                         strict TOML config, path resolution, duration/size parsing
  domain/                         plain Go types and rules: Dataset, inference, transforms, RenderSpec
  storage/                        delimited input, read-only SQLite source, disposable SQLite cache
  render/                         pure chart layout, glyphs, ANSI theme and locale number formatting
  ui/                             Bubble Tea single-screen root: app.go, keys.go, styles.go, commands.go
    testdata/                     TUI golden files (updated with -update-tui)
  i18n/                           bundle setup plus active.<lang>.toml messages (go:embed)
  db/                             sqlc-generated cache code (never edited by hand)
  testutil/                       golden helper shared by render/TUI tests
db/migrations/                    forward-only goose SQL migrations (schema sqlc reads)
db/queries/                       one .sql file per aggregate (cache queries)
sqlc.yaml                         sqlite engine, schema, queries, output package
testdata/                         sample inputs; render/TUI goldens live in internal/*/testdata
docs/                             static product site and assets, served by GitHub Pages
docs/assets/vendor/               browser-only Chart.js, SheetJS, sql.js, jsPDF and licenses
```

Dependency direction: `cmd → ui → render → domain` and `cmd → storage → domain`; `cmd → config` and
`cmd → i18n` are edges. `domain` never imports Bubble Tea or Lip Gloss; `render` never imports Lip Gloss or the UI;
`storage` depends on `domain` ports only. Screens never import each other and talk through messages handled by
`ui/app.go`.

## Conventions

- One component per folder or file; each one exposes `New(...) Model`, `Init() tea.Cmd`, `Update(tea.Msg) (Model, tea.Cmd)` and `View() string`.
- `View` is pure: it reads the model and returns a string. No IO, no time, no randomness in `View` or `Update`.
- Every side effect is a `tea.Cmd` returning a typed message; errors come back as `errMsg{err}` and are shown, never `panic`.
- Children are updated by the parent forwarding messages; the parent batches child commands with `tea.Batch`.
- `tea.WindowSizeMsg` is stored in the root and passed down; components never assume a terminal size.
- Styles and colors only from `ui/styles.go`; use `lipgloss.AdaptiveColor` so light and dark terminals both work.
- Key bindings are `key.Binding`s with help text; the help bubble shows them, so every key is discoverable.
- The current TUI is one screen in `internal/ui`; split it into `ui/screens/plot/{model,update,view}.go` only when a second screen appears.
- Cache connections: `journal_mode=WAL`, `foreign_keys=ON`, `busy_timeout=250`; keep writes short and best-effort.
- Cache reads never create or migrate the database; an absent/uninitialized cache is a miss. Writes run after the
  first frame, are user-only (`0700`/`0600`) and never change output or the exit code.
- The source database is read-only (`mode=ro`, `query_only=ON`, `busy_timeout=5000`); `pl` never writes to it.
- Write cache queries in `db/queries/`, run `sqlc generate`, commit the generated code; never edit `internal/db/`.
- Name queries by intent (`GetCacheEntry`) and annotate with `:one`, `:many`, `:exec`.
- Cache tables are `STRICT` where supported and use forward-only goose migrations; runtime applies the same
  embedded statements with `PRAGMA user_version`, not a CGO goose driver.
- Messages have stable IDs (`cli.error.config`) and a `description` for translators; the `en` file is the source.
- One `*i18n.Localizer` is created per process from `PL_LANG` > `LC_ALL` > `LC_MESSAGES` > `LANG` > `en`; there is no HTTP request and no global current language.
- Call sites use `Localizer.T(id, data)`; the text is not formatted before translation. Missing IDs fall back to the ID and log at debug level, never panic.
- Plurals use `one` / `other` (and more forms where the language needs them) in the message file.
- Number, currency and date formatting use `golang.org/x/text/message` printers for the same language tag.
- Errors for users are translated at the CLI/TUI edge; internal errors and logs stay in English.
- Table-driven: `tests := []struct{ name string; ...; want ... }{...}` and `t.Run(tt.name, func(t *testing.T) { ... })`.
- `require` for conditions that make the rest meaningless (errors, nil), `assert` for independent checks; expected first, actual second.
- Mark helpers with `t.Helper()`, use `t.Cleanup` for teardown, `t.TempDir()` for files and `t.Setenv` for environment.
- Parallel tests call `t.Parallel()` and capture the loop variable correctly for the Go version in `go.mod`.
- Depend on small interfaces defined by the consumer and use hand-written fakes; mocks (`testify/mock`) only when behaviour must be verified.
- Time comes from an injected clock, never `time.Now()` inside logic under test.
- Golden files are updated with an explicit flag and the diff is reviewed.
- Version, commit and date are injected with `-ldflags "-X main.version={{.Version}}"`; `--version` prints them.
- Releases are created by CI on a pushed tag; nobody publishes from a laptop.
- CI runs exactly the commands in the Commands table; no CI-only scripts.
- Third-party actions are pinned to a full commit SHA; `permissions:` default to `contents: read`.
- `concurrency` cancels superseded runs on the same branch; `main` is protected and requires a green CI.
- Hooks run only the fast checks on staged files; tests stay in CI.
- Hooks run the same commands as the Commands table.
- Related packages update together (framework + its plugins, test tooling, linters).
- Patch updates and lockfile maintenance automerge only when CI is green; minor/major are reviewed.
- A major update gets its own pull request with the migration notes linked.

## Commands

| Task | Command |
| --- | --- |
| Run on a file | `go run ./cmd/pl data.csv` |
| Run on a pipe | `printf 'a,b\n1,2\n' \| go run ./cmd/pl` |
| Run the TUI | `go run ./cmd/pl --tui data.csv` |
| Build | `CGO_ENABLED=0 go build -o pl ./cmd/pl` |
| Test | `go test ./...` |
| Test race | `go test -race ./...` |
| Update render goldens | `go test ./internal/render -update` |
| Update TUI goldens | `go test ./internal/ui -update-tui` |
| Lint | `golangci-lint run` |
| Format | `gofmt -l -w .` |
| Vet | `go vet ./...` |
| Generate cache code | `sqlc generate` |
| New cache migration | `goose -dir db/migrations create <name> sql` |
| Apply cache migration (authoring) | `goose -dir db/migrations sqlite3 /tmp/pl-cache.db up` |
| Debug log | `DEBUG=1 go run ./cmd/pl data.csv`, then read `$XDG_CACHE_HOME/pl/debug.log` |
| Inspect cache (Linux) | `sqlite3 ~/.cache/pl/cache.db` |
| Release dry run | `goreleaser release --snapshot --clean` |
| Coverage | `go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out` |
| Install hooks | `lefthook install` |
| Serve the site | `python3 -m http.server -d docs 8765` |

## Testing

- Domain and storage: table-driven unit tests, no terminal involved.
- `Update` functions: call them directly with a message and assert on the returned model and the command's message.
- Whole program: `teatest.NewTestModel` with a fixed size (`teatest.WithInitialTermSize(80, 24)`), send keys, wait for output, compare the final output with a golden file.
- Force a stable color profile in tests (`lipgloss.SetColorProfile(termenv.Ascii)`) so golden files do not depend on the terminal.
- A bug fix starts with a failing test that reproduces the key sequence.
- Cache tests use temporary directories and the real embedded schema; source tests use a temporary SQLite file in read-only mode.
- A test loads every message file and checks each ID and template variable exists in every language.

## Quality gates

- `gofmt` clean, `golangci-lint run` with zero issues (errcheck, govet, staticcheck, revive, gocritic).
- `go test -race ./...` passes and golden files are committed and up to date.
- The binary starts in under 100 ms and never writes logs to stdout/stderr (logs go to a file).

## Dependencies

- Charm libraries for the UI, `modernc.org/sqlite` for SQLite, `BurntSushi/toml` for config, go-i18n/x-text for localization, and the standard library for the rest.
- Allowed: the table above. Anything else needs a reason in the PR.
- The browser playground's Chart.js, SheetJS, sql.js and jsPDF are vendored under `docs/assets/vendor` with their licenses; they are not Go dependencies and must never be imported by `cmd/` or `internal/`.
- Do NOT use: tview/termui/gocui alongside Bubble Tea (two UI models), `fmt.Print` while the program runs (corrupts the screen), global mutable state shared between components, goroutines that write to the model directly (send a message with `p.Send` or a command instead).

## Rules for agents

- Never do IO, sleep or read the clock in `Update` or `View`: return a `tea.Cmd` that produces a message.
- Never mutate a model from a goroutine; results always come back as messages.
- Never print to stdout/stderr while the program runs; log to the file set up with `tea.LogToFile`.
- Every new style or color goes in `ui/styles.go`; never inline `lipgloss.NewStyle()` in a view.
- Every new key gets a `key.Binding` with help text and appears in the help view.
- Handle `tea.WindowSizeMsg` in every component that lays out content; test at 80×24 and a narrow width.
- Keep `domain` free of UI imports; a screen calls the domain only through commands in `ui/commands.go`.
- Every new screen gets a teatest golden test; regenerate golden files only when the change is intended and say so.
- Quit cleanly on `ctrl+c` and `q`, restoring the terminal (no `os.Exit` inside the program).
- Never commit the `.db`, `-wal` or `-shm` files; add them to `.gitignore`.
- Never hold a cache write transaction open; cache contention is handled with a short busy timeout and skipped.
- Always bind parameters; never concatenate user input into SQL.
- Never edit files in `internal/db/`; change the `.sql` and regenerate.
- CI fails if `sqlc generate` leaves a diff.
- Cache SQL lives in sqlc; source SQL is the user-provided read-only query and is never concatenated by `pl`.
- Never hardcode a user-facing string in handlers; add a message ID and translate it for every language.
- Never concatenate translated fragments; use `TemplateData`.
- Never panic on a missing translation; fall back to the default language and log it.
- Every bug fix and new function gets a test that fails without the change.
- Never delete or `t.Skip` a failing test to get green; fix the cause.
- Never rely on `time.Sleep` for synchronization; use channels, `sync.WaitGroup` or `Eventually` with a timeout.
- Never hardcode the version string: it comes from ldflags.
- `goreleaser check` must pass after any change to `.goreleaser.yaml`.
- Never weaken or skip a CI step to make a change pass.
- A new quality gate is added to the Commands table and to `ci.yml` in the same change.
- Never put secrets in workflow files; use repository or environment secrets.
- Never bypass hooks with `--no-verify`.
- Commit messages follow Conventional Commits (`feat(scope): …`, `fix(scope): …`).
- Never pin a dependency to an old version to silence Renovate without a comment saying why.
- Never mix a dependency update with feature changes in one commit.

## Engineering principles

Rules, not suggestions: every change follows them, and a review checks them. When one has to be broken, say so in the change and why.

### Accessibility

- **WCAG 2.2 AA** is the minimum bar for every screen, not a later phase.
- **Semantic elements first**: `button` for actions, `a` for navigation, headings in order, lists, landmarks; ARIA only when no native element fits.
- **Keyboard**: everything works with the keyboard alone, in a logical tab order, with no traps; custom widgets follow the ARIA Authoring Practices patterns.
- **Visible focus** on every interactive element (never `outline: none` without a replacement); focus moves into dialogs and returns on close.
- **Labels**: every input has a visible label; icon-only buttons have an accessible name; images have alt text (empty for decorative ones).
- **Contrast**: text ≥ 4.5:1 (large text ≥ 3:1), UI components and focus indicators ≥ 3:1; color is never the only signal.
- **Targets** at least 24×24 CSS px (44×44 on touch); content reflows at 320px width and 200% zoom.
- **Motion**: respect `prefers-reduced-motion`; nothing flashes more than 3 times per second; auto-playing content can be paused.
- **Errors** are announced, described in text and tied to their field (`aria-describedby`).
- Review check: axe (or equivalent) reports no violations, the flow was done with keyboard only, and a screen reader announces the new UI meaningfully.

### Clean Architecture

- **Four rings**: entities (enterprise rules) → use cases (application rules) → interface adapters (controllers, presenters, gateways) → frameworks and drivers.
- **Dependency rule**: source code dependencies point inward only; an inner ring never names anything declared in an outer one.
- **One use case per file/class**, named as an action (`PlaceOrder`, `CancelSubscription`), with its own input and output models.
- **DTOs at boundaries**: data crossing a ring is a plain structure defined by the inner ring; entities never leak to controllers, views or the database layer.
- **Gateways are interfaces** declared by use cases and implemented in the adapters ring.
- **Presenters format output** for the delivery mechanism; use cases return data, never HTTP status codes, HTML or JSON.
- **Frameworks are details**: they are plugged in at the outer ring and could be replaced without touching use cases or entities.
- Review check: every new use case has its own file, input/output types and a unit test with fake gateways; no inner file imports an outer package.

### Clean Code

- **Names** reveal intent: a reader knows what a variable holds or a function does without reading its body; no abbreviations, no `data`, `info`, `tmp`, `manager`, `helper`.
- **Small functions**: one thing, one level of abstraction; ~30 lines is a smell, ~60 is a split. At most 3-4 parameters; more becomes an options/params type.
- **Guard clauses**: handle invalid cases first and return early; nesting deeper than 3 levels is refactored.
- **No surprises**: a function named like a query does not mutate; side effects are visible in the name (`save`, `send`, `delete`).
- **No magic values**: numbers and strings with meaning become named constants or configuration.
- **Comments explain why**, never what: the code says what. No commented-out code, no journal comments, no stale TODOs.
- **DRY with judgment**: remove duplicated knowledge, not similar-looking code; wait for the third occurrence before abstracting.
- **Boolean flags** that switch a function's behaviour become two functions.
- Review check: every new name reads as a sentence in its call site; no function in the diff exceeds the limits above without a reason in the change.

### Error handling

- **Never swallow**: every error is handled, returned or logged-and-escalated exactly once; empty catch blocks and ignored results are bugs.
- **Wrap with context**: add what was being done (`load order 42: …`) while keeping the original cause inspectable (wrapping, `cause`).
- **Domain errors are types/values** (`ErrOrderNotFound`, `InsufficientFunds`) declared in the core; infrastructure errors stay in adapters and are translated.
- **Map at the boundary, once**: one place turns errors into HTTP status, exit codes or UI messages; no status codes inside business logic.
- **Fail fast at startup**: missing or invalid configuration stops the process with a clear message instead of failing on first use.
- **No exceptions/panics for control flow**: expected outcomes (not found, invalid input) are values; panics only for programmer errors.
- **User-safe messages**: users see what happened and what to do; stack traces, SQL and internal ids go only to logs.
- **Log where handled**, not at every layer: one log entry per failure, with its context and correlation id.
- **Retries** only for transient errors, with backoff, jitter and a limit; never retry non-idempotent operations blindly.
- Review check: no new code path drops an error; every new error type is mapped at the boundary.

### Hexagonal architecture

- **The core** (domain + application) contains business rules and knows nothing about HTTP, databases, queues, frameworks, ORMs or the UI.
- **Ports are owned by the core**: interfaces declared where they are used (`OrderRepository`, `PaymentGateway`, `Clock`), named in business terms, never after a technology.
- **Adapters live at the edge**: driving adapters (HTTP handlers, CLI, consumers) call use cases; driven adapters (Postgres, Stripe, SMTP) implement ports.
- **Dependency rule**: adapters → application → domain. The core never imports an adapter, a driver or a framework package.
- **No framework types in the core**: no request/response objects, ORM entities, annotations or SDK types cross into it; adapters map to and from core types.
- **One composition root** (`main`/bootstrap) wires concrete adapters into the core; nothing else instantiates infrastructure.
- **Tests through ports**: use cases are tested with in-memory fakes of their ports; adapters get their own integration tests against the real technology.
- Review check: grep the core for driver, framework and SDK imports: there are none; every new adapter implements a port declared in the core.

### One file, one responsibility

- **One file = one responsibility**: one entity, one use case, one handler group, one component. If its description needs "and", it is two files.
- **Size limits**: ~300 lines is a soft limit, ~500 a hard one; a file over it is split by responsibility, not by line count.
- **Folders by feature**, not by technical type: `orders/` holds its entity, use cases, adapters and tests; shared kernels stay small and explicit.
- **Explicit public surface**: each module exposes a small API (index/exports/package-level types); everything else is private and may change freely.
- **No dumping grounds**: no `utils`, `helpers`, `common`, `misc` files; a helper lives next to its only user or in a module named for what it does.
- **High cohesion, low coupling**: things that change together live together; modules talk through interfaces or events, never through each other's internals.
- **Replaceable modules**: deleting or swapping a module touches its folder and the composition root, nothing else.
- **No cycles**: module dependencies form a DAG; a cycle is broken by extracting the shared concept or inverting a dependency.
- Review check: each new file can be described in one sentence; no import reaches into another module's private files.

### Performance

- **Measure first**: optimize only with a profile, benchmark or production metric that shows the problem; record the before and after numbers.
- **Budgets**: explicit targets (p95 latency per endpoint, bundle size, startup time, memory) checked in CI or monitoring.
- **No N+1**: data for a list is loaded in one query or a batch (joins, `IN`, dataloaders); queries inside loops are bugs.
- **Bounded work**: every list is paginated, large payloads are streamed, unbounded reads (whole tables, whole files) are not allowed.
- **Indexes** match the queries; every new query is checked with its plan on realistic data.
- **Caching** has an owner, a key, a TTL and an invalidation strategy; never cache without knowing how it becomes stale.
- **Timeouts everywhere**: every network call, query and lock has a timeout and cancellation propagates (context, abort signals).
- **Async for slow work**: emails, reports and third-party calls that the user need not wait for go to a queue.
- **No premature optimization**: clear code first; micro-optimizations need a measurement that justifies the lost readability.
- Review check: no new query in a loop, no unbounded list, no network call without a timeout.

### Security

- **Validate at every boundary**: input from users, APIs, files and queues is parsed into typed values with allowlists, size and range limits; reject, never sanitize-and-hope.
- **Injection**: parameterized queries and safe APIs only; never build SQL, shell commands, paths or HTML by string concatenation with input.
- **Authorization on every request**: check that this user may do this action on this resource (object-level), server-side, deny by default.
- **Authentication**: proven libraries, hashed passwords (argon2id/bcrypt), short-lived tokens, secure cookies (`HttpOnly`, `Secure`, `SameSite`), CSRF protection for cookie sessions.
- **Secrets** never in code, logs, URLs or the repository; they come from the environment or a secret manager and are rotatable.
- **Least privilege**: database users, service accounts, tokens and containers get only the permissions they need.
- **Output encoding** by context (HTML, attribute, URL, JS); security headers (CSP, HSTS) on web apps.
- **No PII or secrets in logs**; personal data is minimized, encrypted in transit (TLS) and at rest where required.
- **Dependencies** are pinned and scanned (Dependabot, `npm audit`, `govulncheck`, `cargo audit`); known-vulnerable versions block the build.
- Review check: every new endpoint has an authorization check and input validation; no new string-built query or command.

### SOLID

- **Single responsibility**: a module, type or function has one reason to change. If describing it needs "and", split it.
- **Open/closed**: add behaviour by adding code (a new implementation, a new strategy, a new handler), not by growing `switch`/`if` chains in existing code.
- **Liskov substitution**: every implementation of an interface honours its full contract (inputs, outputs, errors, side effects); no implementation panics or ignores a method it "does not support".
- **Interface segregation**: interfaces are small and owned by the consumer; a consumer never depends on methods it does not call.
- **Dependency inversion**: business code depends on interfaces it defines; concrete adapters (DB, HTTP, queues, clocks, random) are injected from the composition root.
- Review check: no constructor creates its own infrastructure; no type takes more than ~5 dependencies (a sign it does too much).

### Testing

- **Pyramid**: many fast unit tests, fewer integration tests against real dependencies (containers), a few end-to-end tests of critical flows.
- **Test behaviour, not implementation**: assert on outputs and observable effects through public APIs; refactoring must not break tests.
- **Arrange, Act, Assert**: one behaviour per test, named after it (`rejects_order_without_items`); table-driven for variations.
- **Deterministic**: no `sleep`, no real clock, randomness or network in unit tests; inject clocks, id generators and seeds.
- **Fakes over mocks** for ports you own (in-memory repository); mocks only to assert interactions that are the behaviour (an email was sent).
- **Isolated**: tests do not depend on order or shared state; each creates its data and cleans up (transactions rolled back, temp dirs).
- **Every bug gets a regression test** that fails before the fix and passes after it.
- **Coverage is a signal, not a target**: uncovered critical paths matter; chasing a percentage with trivial tests does not.
- Review check: every changed behaviour has a test in the same change; the suite passes repeatedly and in parallel.
