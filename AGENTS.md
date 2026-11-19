# AGENTS.md: plot

## Project

`pl` is a fast terminal data plotter written in Go with the Charm stack (Bubble Tea, Lip Gloss, Bubbles). It reads tabular data from stdin pipes (`psql --csv -c "..." | pl`), CSV files, or a configured database query, infers columns and chart type automatically, and renders 2D plots, bar graphs, donut and pie charts as terminal characters. CLI mode is the default and is pipe-friendly; `--tui` opts into an interactive, animated view. Users are shell users and CSV analysts who want a chart without leaving the terminal.

## Read first

- [STACK.md](STACK.md): the stack, the project layout and the engineering rules every change follows.
- [DESIGN.md](DESIGN.md): the design system: tokens, components and UI rules.
- [SPEC.md](SPEC.md): what is being built: requirements, acceptance criteria and open questions.

Read the code around a change before writing: `cmd/pl` is the entry point, `internal/domain` owns the rules, `internal/storage` parses input and owns the disposable cache, `internal/render` draws, `internal/ui` holds the TUI root, keys, styles and messages, and `internal/config` loads strict TOML. A `go.mod` exists; no `Makefile` yet, so use the raw commands below.

## Commands

| Task | Command |
| --- | --- |
| Install dependencies | `go mod download` |
| Build everything | `CGO_ENABLED=0 go build ./...` |
| Build the binary | `CGO_ENABLED=0 go build -o pl ./cmd/pl` |
| Run on a file | `go run ./cmd/pl data.csv` |
| Run on a pipe | `printf 'a,b\n1,2\n' \| go run ./cmd/pl` |
| Run the TUI | `go run ./cmd/pl --tui data.csv` |
| All tests | `go test ./...` |
| Single test | `go test ./internal/domain -run TestInferClassifiesColumnsAndValues -v` |
| Race detector | `go test -race ./...` |
| Update render goldens | `go test ./internal/render -update` |
| Update TUI goldens | `go test ./internal/ui -update-tui` |
| Vet | `go vet ./...` |
| Format | `gofmt -l -w .` |
| Lint | `golangci-lint run` (v2) |
| Generate sqlc code | `sqlc generate` |
| New migration | `goose -dir db/migrations create <name> sql` |
| Apply migrations | `goose -dir db/migrations up` |
| Install git hooks | `lefthook install` |
| Release dry run | `goreleaser release --snapshot --clean` |

`TODO:` the Makefile/scripts do not exist yet; once added, prefer its targets (`make test`, `make lint`) over the raw commands above and update this table. The goose dialect is `sqlite3`; the runtime cache path is `--cache-dir` > `PL_CACHE_DIR` > `cache.dir` > `os.UserCacheDir()/pl`.

## Layout

```
cmd/pl/            entry point: flags, source selection, CLI pipeline, exit codes
internal/domain/    plain Go types and rules (Dataset, Column, RenderSpec, inference); no Bubble Tea/Lip Gloss
internal/config/    strict TOML config, path resolution, duration/size parsing
internal/storage/   delimited input (stdin/CSV), read-only SQLite source, disposable SQLite cache
internal/render/    pure chart layout, glyphs and ANSI; no Bubble Tea, no Lip Gloss
internal/ui/        app.go root model, keys.go, styles.go, msgs.go, commands.go
internal/ui/testdata/  TUI golden files (updated with -update-tui)
internal/i18n/      bundle setup plus active.<lang>.toml messages (go:embed)
internal/testutil/  golden helper; builders and fakes arrive with more tests
internal/db/        sqlc-generated cache code (never edited by hand)
db/migrations/      forward-only goose SQL migrations (schema sqlc reads)
db/queries/         one .sql file per aggregate (cache queries)
sqlc.yaml           sqlite engine, schema, queries, generated package
testdata/           sample inputs; render and TUI goldens live in internal/*/testdata
.github/workflows/  CI: lint, test, build (future)
```

Dependency direction: `cmd → ui → render → domain` and `cmd → storage → domain`. `domain` never imports Bubble Tea or Lip Gloss; `storage` depends on `domain` ports only; screens never import each other and talk through messages handled by `ui/app.go`.

## Conventions

- File organisation: one file = one responsibility. The current TUI is a single screen in `internal/ui`; split it into `screens/plot/{model,update,view}.go` when a second screen appears.
- All styling lives in `internal/ui/styles.go`; all bindings in `internal/ui/keys.go`; every side effect is a `tea.Cmd` constructor in `commands.go`. No IO, timers or HTTP inside `Update` or `View`.
- Colors are adaptive ANSI roles with a light and dark variant; honor `NO_COLOR`, `TERM=dumb` and non-TTY output. Never paint the app background.
- Errors: wrap with `%w`, return instead of panicking, and surface user-facing failures as a single line on stderr. Database driver errors are printed verbatim. stdout belongs to plot output and the UI; when `DEBUG` is set, `log/slog` writes only to `os.UserCacheDir()/pl/debug.log`, never to stdout or stderr.
- i18n: no hard-coded user-facing strings in Go; ids and description go in `internal/i18n/active.en.toml`, translations in `active.<lng>.toml`, both embedded.
- Tests sit next to the code in the same package (`<pkg>`), or `<pkg>_test` for the public API. Table-driven subtests with `stretchr/testify` `require`/`assert`. Render goldens use `internal/testutil`, TUI goldens use `teatest` or `-update-tui`, fixtures in `testdata/`.
- Cache writes are best-effort and happen after the first frame, inside a `tea.Cmd` in TUI mode; they never change output or the exit code. Config precedence is flag > environment > config > default; paths in TOML resolve relative to the config file.
- Generated code (`internal/db/`) is regenerated with `sqlc generate`, never edited by hand, and committed.
- Commits follow Conventional Commits (`feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`); Lefthook enforces format, lint and the commit-msg check.

## Workflow

1. Find the governing requirement in [SPEC.md](SPEC.md) (FR-n, NFR-n); if the task contradicts it, stop and ask.
2. Read the code around the change and the layer it belongs to; check the dependency direction before adding an import.
3. Reproduce the behaviour with the real command (`go run ./cmd/pl ...` or a failing test) before editing.
4. Add or adjust a test first: unit test next to the code, golden test for anything rendered.
5. Make the minimal change in the correct layer; keep `domain` free of UI imports and side effects inside `tea.Cmd`s.
6. Run `gofmt -l -w .`, `go vet ./...`, `golangci-lint run`, `go test ./...`; regenerate sqlc code if a query or migration changed.
7. Update the affected document (STACK/DESIGN/SPEC) when behaviour or commands change.
8. Commit one logical change with a Conventional Commits message.

## Boundaries

- Never commit secrets, DSNs, credentials or `.env` files; database config comes from the environment/config dir, never from source.
- Never hand-edit `internal/db/`, `go.sum` or generated goldens; regenerate them and review the diff. A new golden file must be justified in the commit. Runtime cache migrations live in `internal/storage/cache.go` and must stay in sync with `db/migrations/`.
- Never add a dependency outside the STACK.md list, or a chart/plotting library, without asking first.
- Never import Bubble Tea or Lip Gloss from `internal/domain`, import one screen from another, or perform blocking work inside `Update`/`View`.
- Never write logs or debug output to stdout; it corrupts the chart and the TUI.
- Never change established CLI flags, exit codes or output format silently; those are contract (SPEC FR-1, FR-2) — ask first.
- Never run destructive migrations (dropping columns/tables) or rewrite past migrations; add a new numbered forward migration.
- Ask before touching `.github/workflows/`, `.goreleaser.yaml`, `lefthook.yml`, `renovate.json`, `sqlc.yaml`, or anything release-related.
- No unrelated rewrites, reformatting of untouched files, or drive-by renames in the same change.

## Definition of done

- [ ] `go build ./...` succeeds and `go vet ./...` is clean.
- [ ] `golangci-lint run` reports no issues.
- [ ] `go test ./...` passes, including any golden test updated intentionally with `-update`.
- [ ] The behaviour matches the FR/NFR it implements, verified by running the real command.
- [ ] `internal/domain` still imports no UI package; side effects still live in `tea.Cmd`s.
- [ ] Generated code and goldens were regenerated, not edited by hand.
- [ ] Docs (STACK/DESIGN/SPEC) still true; changed commands are reflected in this file.
- [ ] Diff is minimal: no unrelated files, no leftover debug output, no secrets.
- [ ] Commit message follows Conventional Commits.
