# STACK.md: Go TUI (Bubble Tea)

## Summary

A TUI (terminal user interface) application in Go built on Charm's Bubble Tea, which follows the Elm architecture: a
`Model` holds state, `Update` turns messages into a new model plus commands, `View` renders a string. Side effects
(IO, timers, HTTP) run only inside `tea.Cmd`s and come back as messages. The screen is a tree of small components, each
with its own model, messages and view; the domain logic lives outside the UI and is plain, testable Go.

## Stack

| Layer | Choice | Why |
| --- | --- | --- |
| Language | Go 1.25+ | One static binary, fast startup, trivial cross-compilation |
| TUI runtime | `github.com/charmbracelet/bubbletea` | Elm architecture: predictable state, side effects as commands |
| Components | `github.com/charmbracelet/bubbles` (list, table, textinput, viewport, spinner, help, key) | Tested building blocks with key bindings and help |
| Styling | `github.com/charmbracelet/lipgloss` | Declarative styles, layout (`JoinHorizontal`, `Place`), adaptive colors |
| Forms | `github.com/charmbracelet/huh` | Prompts and multi-step forms that embed in a Bubble Tea program |
| Markdown | `github.com/charmbracelet/glamour` | Renders help, docs and previews in the terminal |
| CLI entry | `github.com/spf13/cobra` (or `flag` for one command) | Subcommands, flags, completions |
| Config | TOML/YAML in `os.UserConfigDir()` + env overrides | Follows the platform's conventions |
| Logging | `log/slog` to a file (`tea.LogToFile`) | stdout belongs to the UI |
| Testing | `testing` + `github.com/charmbracelet/x/exp/teatest` (golden files) | Whole-program tests with final output snapshots |
| Lint | golangci-lint v2 | One config, many linters |
| Release | GoReleaser (`.goreleaser.yaml`): cross-compiled archives, checksums, SBOM, Homebrew tap, on every `v*` tag | One tag = a complete, reproducible release |
| Database | SQLite (one file, WAL mode, `foreign_keys=ON`, versioned migrations) | Zero operations: no server, trivial backups, fast for one writer |
| Data access | sqlc (SQL queries in `.sql` files compiled to typed Go, `pgx` driver, migrations with `goose`) | Plain SQL reviewed as SQL, generated code with no runtime reflection |
| Internationalization | go-i18n v2 (`nicksnyder/go-i18n`, TOML/JSON message files embedded with `embed`, `golang.org/x/text/language`) | CLDR plurals and template data with no code generation |
| Unit tests | Go `testing` with table-driven subtests and `stretchr/testify` (`require`, `assert`) | Standard tooling plus readable assertions |
| CI | GitHub Actions (`.github/workflows/ci.yml`) on every pull request and push to `main` | Runs the same quality gates as local, next to the code review |
| Hooks | Lefthook (`lefthook.yml`): pre-commit format + lint on staged files, commit-msg Conventional Commits check | Problems caught before they reach CI, in parallel and in milliseconds |
| Updates | Renovate (`renovate.json`): grouped weekly updates, automerge for patch and lockfile maintenance | Dependencies stay current in small, reviewable steps |

## Project layout

```
cmd/app/main.go                  flags/cobra, config load, builds the root model, tea.NewProgram(...).Run()
internal/
  domain/                        plain Go: types and rules, no Bubble Tea imports
    task.go                      Task entity and its invariants
    store.go                     TaskStore port (interface)
  storage/
    file_store.go                implements TaskStore (JSON/SQLite on disk)
  ui/
    app.go                       root model: owns the active screen, routes messages, global keys (quit, help)
    keys.go                      key.Binding map shared by help views
    styles.go                    every lipgloss.Style and color in one place (adaptive light/dark)
    msgs.go                      app-wide messages (errMsg, tasksLoadedMsg, windowSizeMsg handling)
    commands.go                  tea.Cmd constructors that call the domain/storage (loadTasks, saveTask)
    screens/
      list/
        model.go                 list screen: Model, Init
        update.go                Update: messages → new state + commands
        view.go                  View: pure rendering from the model
      detail/{model,update,view}.go
      edit/form.go               huh form embedded as a component
    components/
      statusbar.go               one reusable component per file
testdata/                        teatest golden files (*.golden)
```

Dependency direction: `cmd → ui → domain ← storage`. `domain` never imports Bubble Tea or Lip Gloss; screens never
import each other: they talk through messages handled by `app.go`.

```
migrations/                    numbered, forward-only SQL migrations
data/app.db                    the database file (git-ignored; with its -wal and -shm files)
```

```
sqlc.yaml                      engine, schema (migrations folder), queries folder, output package
db/migrations/                 goose SQL migrations (the schema sqlc reads)
db/queries/                    one .sql file per aggregate, each query named with `-- name: GetUser :one`
internal/db/                   generated code (never edited by hand), committed
```

```
internal/i18n/bundle.go        bundle setup, language matcher, Localizer per request
internal/i18n/active.en.toml   source messages (id, description, one/other)
internal/i18n/active.<lng>.toml  translations per language, loaded through go:embed
```

```
internal/<pkg>/<file>_test.go  tests next to the code, same package for white-box, `<pkg>_test` for the public API
internal/testutil/             builders, fixtures, fake clock and fake clients shared by tests
testdata/                      golden files and sample inputs (ignored by the Go tool)
```

```
.goreleaser.yaml               builds (CGO_ENABLED=0, linux/darwin/windows × amd64/arm64), archives, checksum, brews
```

```
.github/workflows/ci.yml       jobs: lint (format + lint + typecheck), test, build; dependency cache; concurrency per branch
```

```
lefthook.yml                   pre-commit: format + lint ({staged_files}); commit-msg: conventional commit check
```

```
renovate.json                  extends config:recommended; schedule; packageRules: groups, automerge patch
```

## Conventions

- One component per folder or file; each one exposes `New(...) Model`, `Init() tea.Cmd`, `Update(tea.Msg) (Model, tea.Cmd)` and `View() string`.
- `View` is pure: it reads the model and returns a string. No IO, no time, no randomness in `View` or `Update`.
- Every side effect is a `tea.Cmd` returning a typed message; errors come back as `errMsg{err}` and are shown, never `panic`.
- Children are updated by the parent forwarding messages; the parent batches child commands with `tea.Batch`.
- `tea.WindowSizeMsg` is stored in the root and passed down; components never assume a terminal size.
- Styles and colors only from `ui/styles.go`; use `lipgloss.AdaptiveColor` so light and dark terminals both work.
- Key bindings are `key.Binding`s with help text; the help bubble shows them, so every key is discoverable.
- New screen = a folder under `ui/screens/` with model/update/view + its messages + a route in `app.go` + a teatest test.
- Path from configuration (`DATABASE_PATH`); the directory is a persistent volume in containers.
- On every connection: `PRAGMA journal_mode=WAL`, `PRAGMA foreign_keys=ON`, `PRAGMA busy_timeout=5000`.
- One writer at a time: keep write transactions short and never hold one across a network call.
- Types are loose: add `CHECK` constraints and `STRICT` tables where the driver supports them.
- Back up with `sqlite3 data/app.db ".backup out.db"`, never by copying a live file.
- Write the query in `db/queries/`, run `sqlc generate`, commit the generated code.
- Name queries by intent (`ListOrdersByCustomer`) and annotate with `:one`, `:many`, `:exec`.
- Repositories wrap the generated `Queries`; handlers never import `internal/db` directly.
- Transactions use `pool.Begin` and `queries.WithTx(tx)`.
- Use `sqlc.arg` / `sqlc.narg` for named and nullable parameters.
- Messages have stable IDs (`checkout.paid`) and a `description` for translators; the `en` file is the source.
- A `*i18n.Localizer` is created per request from `Accept-Language` / the user setting with `language.NewMatcher`; no global current language.
- Call sites use `localizer.Localize(&i18n.LocalizeConfig{MessageID: ..., TemplateData: ..., PluralCount: n})`; the text is not formatted before translation.
- Plurals use `one` / `other` (and more forms where the language needs them) in the message file.
- Number, currency and date formatting use `golang.org/x/text/message` printers for the same language tag.
- Errors for users are translated at the edge (handler); internal errors and logs stay in English.
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
| Run | `go run ./cmd/app` |
| Build | `go build -o bin/app ./cmd/app` |
| Test race | `go test -race -count=1 ./...` |
| Update golden files | `go test ./internal/ui/... -update` |
| Lint | `golangci-lint run` |
| Format | `gofmt -w .` |
| Debug log | `DEBUG=1 go run ./cmd/app` then `tail -f debug.log` |
| Release (dry run) | `goreleaser release --snapshot --clean` |
| Shell | `sqlite3 data/app.db` |
| Backup | `sqlite3 data/app.db ".backup backup.db"` |
| Generate | `sqlc generate` |
| Vet | `sqlc vet` |
| Migrate | `goose -dir db/migrations postgres "$DATABASE_URL" up` |
| Migration new | `goose -dir db/migrations create <name> sql` |
| i18n merge | `goi18n merge -format toml -outdir internal/i18n internal/i18n/active.*.toml` |
| Coverage | `go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out` |
| Check config | `goreleaser check` |
| Install hooks | `lefthook install` |

## Testing

- Domain and storage: table-driven unit tests, no terminal involved.
- `Update` functions: call them directly with a message and assert on the returned model and the command's message.
- Whole program: `teatest.NewTestModel` with a fixed size (`teatest.WithInitialTermSize(80, 24)`), send keys, wait for output, compare the final output with a golden file.
- Force a stable color profile in tests (`lipgloss.SetColorProfile(termenv.Ascii)`) so golden files do not depend on the terminal.
- A bug fix starts with a failing test that reproduces the key sequence.
- Tests use a temporary file or `:memory:` with the real migrations applied, so schema and constraints are the production ones.
- A test loads every message file and checks each ID and template variable exists in every language.

## Quality gates

- `gofmt` clean, `golangci-lint run` with zero issues (errcheck, govet, staticcheck, revive, gocritic).
- `go test -race ./...` passes and golden files are committed and up to date.
- The binary starts in under 100 ms and never writes to stdout outside the program (logs go to a file).
- `go test -race ./...` passes.

## Dependencies

- Charm libraries for the UI, the standard library for everything else.
- Allowed: the table above. Anything else needs a reason in the PR.
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
- Never open the same database from several processes that write heavily; if that is needed, say so and propose PostgreSQL.
- Always bind parameters; never concatenate user input into SQL.
- Never edit files in `internal/db/`; change the `.sql` and regenerate.
- CI fails if `sqlc generate` leaves a diff.
- Never build SQL strings by concatenation outside sqlc; use parameters.
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
