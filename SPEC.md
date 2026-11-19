# SPEC.md: `pl` — terminal data plotter

**Status:** v1 draft, 2026-11-18. Aligned with [STACK.md](STACK.md), [DESIGN.md](DESIGN.md) and
[AGENTS.md](AGENTS.md). The flags, exit codes and output rules in §9–§11 are contract: changing them requires a
spec change, not a drive-by edit. §15 lists the small STACK/AGENTS updates needed to remove the last contradictions.

**At a glance:** CLI-first; read-only SQLite source and SQLite/sqlc cache; line/bar/pie/donut with a data-driven
default; exit codes `0/1/2/3/130/141`; no network, no CGO.

## 1. Summary

`pl` is a fast, CGO-free, single-binary terminal plotter written in Go. It reads tabular data from stdin, one or
more delimited files, or a read-only SQLite query; infers columns; optionally selects, renames, sorts and filters;
and renders a line plot, bar chart, donut or pie as terminal characters. CLI mode is the default and pipe-friendly;
`--tui` opts into a Bubble Tea view with a framed chart, a help line and a short first-paint animation.

It never opens a network connection, never paints the terminal background, and writes nothing but the chart to
stdout. Diagnostics go to stderr (CLI) or the status/help area (TUI); debug logs go to a file.

## 2. Problem

Developers and data-adjacent users already have data in a shell, a CSV file or a database, but going from that data
to a picture means leaving the terminal (spreadsheets, notebooks, browser chart tools). Existing terminal plotters
require flags for every column and every axis, so a quick "what does this query look like?" costs more typing than
the query itself.

There is no single small binary that accepts a pipe, guesses the columns correctly, draws a readable chart
immediately, and can escalate to an interactive view when the user wants to explore.

## 3. Goals

- **One command, zero flags:** `cat data.csv | pl` and `pl data.csv` render a correct, readable chart without
  configuration.
- **Four chart families:** line (2D plot), bar (graph), donut and pie.
- **Three input modes:** piped stdin, delimited files (including multiple files), and a read-only SQLite query.
- **Useful inference:** label/value classification, chart-type default, null handling and series limits that are
  correct for the common case, overridable per column.
- **Pipe-friendly output:** exactly one static frame, plain text when stdout is not a TTY, no cursor movement, no
  escape noise when redirected, quiet `SIGPIPE` behavior.
- **Optional TUI:** framed chart, keyboard help, resize and a short first-paint animation; reduced-motion and
  `NO_COLOR` respected; terminal restored on exit.
- **Fast:** first frame under the NFR-1 budget, warm-cache re-render under 50 ms, bounded memory.
- **Accessible by construction:** Unicode with ASCII fallback, color-blind-safe series, legend and markers that do
  not rely on color alone.
- **Localized:** every user-facing string has a message ID; numbers follow the active locale.

## 4. Non-goals

- Not a dashboard, multi-panel report or multi-chart tool: one dataset and one chart per invocation.
- Not a statistics package: no regression, fitting, aggregation engines, pivot tables, trendlines or forecasting.
- Not a GUI, browser, image, SVG, HTML or PDF exporter; output is terminal characters and ANSI attributes.
- Not a general database client or SQL IDE; v1 reads SQLite read-only. PostgreSQL/MySQL users pipe `psql --csv` or
  `mysql --batch` output instead.
- Not a replacement for `youplot`'s exact CLI surface; it is inspiration only.
- No 3D plots, maps, heatmaps, scatter matrices or real-time streaming charts.
- No JSON/JSONL/Parquet/Excel input and no compression handling in v1; `zcat file.csv.gz | pl` works.
- No multi-series bar, grouped/stacked bars or multi-series pie/donut in v1.
- No config profiles, multiple simultaneous database connections or saved TUI state.

## 5. Users and scenarios

**Persona A — Shell user.** Has data in a pipe, wants a chart now, does not want to read docs.

1. Runs `psql -c "select day, signups from metrics" --csv | pl`.
2. `pl` detects the label column (`day`) and the numeric series (`signups`), picks the default chart, renders
   immediately.
3. Leaves the output on screen or redirects it to a file; no animation, no terminal reset noise.

**Persona B — CSV analyst.** Has `sales.csv` on disk.

1. Runs `pl sales.csv`.
2. `pl` reads the header, infers types and renders a chart.
3. Narrows down: `--columns revenue --label month --rename revenue=Revenue --sort revenue:desc`,
   `--filter 'region=EU'`, `--title 'EU revenue'`.
4. Switches to `--tui` to explore chart types and quits without writing anything to disk.

**Persona C — TUI explorer.** Wants animation and interaction.

1. Runs `pl --tui sales.csv`.
2. Navigates chart types with `tab`, scrolls with arrows/`hjkl`, presses `?` for help, resizes the terminal and
   sees the chart re-lay out.
3. Quits with `q` or `Ctrl+C`; the terminal is restored and the shell prompt is clean.

**Persona D — Accessibility/CI user.** Runs with `NO_COLOR=1`, `TERM=dumb`, an ASCII-only locale or `--ascii`,
pipes the result into `less`/a file, and still gets a readable chart with distinct series.

## 6. Requirements

### 6.1 Input

**FR-1 — stdin ingestion.** The system must read tabular data from stdin when stdin is not a TTY and no positional
file or database source is given.

- Given `printf 'a,b\n1,2\n3,4\n' | pl`, when the process starts, then it renders a chart from those two rows
  without requiring any flag and exits `0`.
- Given stdin is a TTY and no source is given, when the process starts, then it prints usage to stderr and exits
  `2`; it never blocks reading the terminal.
- Given stdin is empty (0 bytes or whitespace only), when the process starts, then it prints `no data on stdin` to
  stderr, exits `1`, and does not hang.
- Given stdin is larger than `PL_MAX_INPUT_BYTES` (default 1 GiB), when reading, then it stops, prints a one-line
  size error and exits `1`.
- Given a positional `-`, when stdin is not a TTY, then stdin is read as one of the inputs; when stdin is a TTY,
  then `-` is a usage error (`2`).
- Given both a piped stdin and a positional file/database source, then the explicit source wins and stdin is not
  read.

**FR-2 — file ingestion.** The system must accept one or more file paths as positional arguments and read them as
delimited text.

- Given `pl sales.csv`, when the file exists and parses, then the chart renders from its rows and the process exits
  `0`.
- Given multiple files, when their field counts and header names match, then their rows are concatenated in argument
  order and rendered as one dataset.
- Given multiple files whose headers or field counts do not match, when parsing, then it prints a one-line error
  naming the first mismatch and exits `3`.
- Given a path does not exist, is a directory, or is unreadable, when the process starts, then it prints a
  single-line error naming the path and exits `3`.
- Given an empty file, when parsing, then it prints `no data in <path>` to stderr and exits `1`.
- A UTF-8 BOM is stripped. Files are read to EOF once; a read error is a source error (`3`). Changes made while the
  file is being read are outside the contract.

**FR-3 — dialect and header detection.** The system must detect the delimiter and whether the first record is a
header, and must allow explicit overrides.

- Delimiters auto-detected: comma, tab, semicolon and pipe. Detection scores the first records for a consistent
  field count greater than one; ties prefer comma, then tab, then semicolon, then pipe.
- `--delimiter` accepts `auto` (default), `comma`, `tab`, `semicolon`, `pipe`, the literal characters `,`, `;`, `|`
  or `\t`, or any other single rune. A multi-rune value is a usage error (`2`).
- Delimiter and header detection run on the first input (the first file, or stdin when it is first); every
  subsequent file must match its field count and header names.
- Header detection: the first record is a header when at least one of its cells fails the type inferred from the
  remaining records, or when all its cells are non-empty text and at least one column below is numeric/time.
  Otherwise the file is headerless.
- `--header` forces header parsing; `--no-header` forces data parsing; passing both is a usage error (`2`).
- Header cells are trimmed; an empty header cell becomes `column_<i>`; duplicate names get `_2`, `_3`, … suffixes
  and one warning. Headerless columns are named `column_1` … `column_n`.
- Quoting follows RFC 4180: double quotes, doubled quotes, embedded delimiters and newlines, CRLF or LF records.
- Ragged records follow the edge-case rule in §12 (pad short rows with nulls, ignore extra fields, warn once);
  they are not a fatal parse error.
- TSV is supported as tab-delimited input. JSON/JSONL are out of scope.
- Aligned human-readable table output (for example `psql` without `--csv`) is not delimited input; use
  `psql --csv` or `psql -A -F,` instead. `mysql --batch` tab-delimited output is auto-detected.

**FR-4 — SQLite source.** The system must execute a read-only SQL query against a SQLite database and plot the
result set exactly as delimited rows.

- `--db <path>` selects a SQLite 3 file; exactly one of `--query <sql>` or `--query-file <path>` supplies one
  statement (a trailing semicolon is allowed). Values may also come from config or environment.
- The database is opened read-only (`mode=ro`) with `PRAGMA query_only=ON` and `busy_timeout=5000`; `pl` never
  writes to the source database. `--db` is a filesystem path, not a URI with parameters.
- The result set's column names become column names (duplicates suffixed as in FR-3); driver values are normalized
  to null, number, time, text or blob, then the normal inference rules in FR-5 apply. BLOB columns are text labels
  (`<blob N bytes>`), never value series.
- Given valid parameters and a query, when run, then rows are plotted exactly as CSV rows would be.
- Given a missing `--db`, missing query, unreadable file, non-SQLite file, SQL error or timeout, then the error is
  reported and the exit code is `2` for a missing parameter and `3` for a source/query failure. Database driver
  messages are printed verbatim; `pl` never wraps or rewrites them.
- `--timeout` (default `10s`) cancels a long query; a timeout is a source error (`3`) with a clear message. Ctrl+C
  cancels and exits `130` in CLI mode (TUI follows FR-11).
- No database credentials exist in v1; the SQLite path is not a secret. Query text and result values are never
  written to logs.

### 6.2 Inference and transformation

**FR-5 — automatic column inference.** The system must classify every column as label (categorical/text) or value
(numeric) and choose one label column and one or more value series without user input.

- Cell classification: null tokens are empty/whitespace, `N/A`, `NA`, `null`, `NULL`, `nil`, `NaN`, `Inf` and `-`;
  numeric cells parse as finite decimal/scientific numbers; time cells match one of the accepted ISO-like layouts
  (RFC 3339, `YYYY-MM-DD`, `YYYY-MM-DD HH:MM:SS`, `YYYY-MM-DDTHH:MM:SS`, `HH:MM`, `HH:MM:SS`); boolean cells match
  `true/false/yes/no/on/off` case-insensitively; everything else is text.
- A column is a value column when at least 80% of its non-null cells are numeric; non-parsing cells become nulls
  and are counted. Otherwise it is a label column. A boolean column is a label in v1.
- An all-null/empty column is excluded with one warning. If no value column remains, `pl` prints the available
  column names and exits `1`.
- Label selection: the first non-empty label column in source order. If several label columns exist, the first is
  used and the others are listed in one warning. If none exists (all columns numeric), the first column is the
  x-axis and the remaining numeric columns are series.
- Numeric x-axis columns are treated as values on the x-axis, not as series; time labels use a time-proportional
  x-axis, categorical labels use ordinal positions.
- Value columns are series in source order. For line charts, at most 8 series are shown; auto-selection takes the
  first 8 and warns, explicit selection of more than 8 is a usage error (`2`).
- Given exactly one non-numeric column followed by numeric columns, when rendered, then the non-numeric column is
  the label and all numeric columns are series (subject to the 8-series rule).
- Given all columns are numeric, when rendered, then the first column is the x-axis and the rest are series.
- Given exactly one numeric column and no label column, an implicit `row` index (1..n) becomes the x-axis and the
  column is the sole series, so `seq 10 | pl` renders a line.
- Given `--label` or `--columns`, the corresponding inference is overridden with no "inference was wrong" warning.
- Given one row, all-identical values, or a zero-span numeric/time range, then the frame is still valid (flat line
  at mid-height, equal bars at a stable height, or a single visible point); no division by zero.
- Nulls are gaps for line charts, absent bars for bar charts, and excluded slices for pie/donut; the null count is
  reported in the title/status line and once in warnings when material.

**FR-6 — column selection.** The system must let the user choose which columns are plotted, by name and by 1-based
position.

- `--columns` takes a comma-separated list of names or 1-based positions; it is repeatable; order is preserved and
  duplicates collapse to the first occurrence.
- Column names match the parsed `name` (including duplicate suffixes), case-sensitively. A token that matches a
  name wins over a position; otherwise a positive integer is a position.
- `--label` selects exactly one label/x column by name or position; it is independent of `--columns`.
- `--label` and `--columns` must be disjoint. Selecting the label/x column in `--columns` is a usage error (`2`),
  even when that column is numeric; use `--label` to choose it as the x-axis.
- Selecting a non-numeric column in `--columns` is a usage error (`2`) that says to use `--label` and lists the
  available names.
- Given `--columns` names a non-existent column, when run, then the error lists the available columns and exits
  `2`.
- Given a subset is selected, when rendered, then only those series appear in the chart and legend.

**FR-7 — rename and title.** The system must let the user rename displayed column labels and set the chart title
without changing the source data.

- `--rename old=new` is repeatable or comma-separated; `old` is a name or 1-based position, `new` is the display
  label. `--title <text>` sets the title.
- Renames are display-only: selection, sorting and filtering always use the original parsed names. `pl --help`
  states this.
- Given a rename maps an existing column to a new label, when rendered, then the new label appears in the axis,
  legend and title as applicable, and the original name is not shown.
- Given a rename targets a missing column, an empty new label, or a duplicate display label, then it is a usage
  error (`2`) listing the available columns.
- Given no `--title`, the default title is `<source> — <chart type>` (`-` in ASCII mode), where source is `stdin`,
  a file basename, or a database basename; multiple files use the first basename plus `(+N more)`. Titles truncate
  with an ellipsis rather than wrapping.

**FR-8 — sort and filter.** The system must sort rows by chosen columns and filter rows before rendering.

- `--sort column[:asc|desc]` is repeatable; sorts are stable and applied in order. `column` is a name or 1-based
  position. `asc` is the default. Nulls sort last in both directions. Missing columns, bad directions or empty
  values are usage errors (`2`).
- `--filter '<column><op><value>'` is repeatable; repeated filters are combined with AND. Operators are `=`, `!=`,
  `<`, `<=`, `>`, `>=` and `contains`. Values are numbers, `null`, quoted strings or bare tokens; spaces around
  symbolic operators are optional, while `contains` must be surrounded by spaces (`name contains Ada`). There is
  no OR, grouping, SQL or function syntax in v1.
- Filtering happens before sorting; both happen after parsing/inference and before selection/rename. Sorting and
  filtering may use columns that are not plotted.
- Comparisons use the column type: numeric for value columns, chronological for time, Unicode code-point order for
  text. `contains` is a case-sensitive substring test. `= null` matches nulls, `!= null` matches non-nulls, and
  other operators never match null.
- `contains` applies to text/time labels; applying it to a numeric value column is a usage error (`2`).
- Given a filter expression excludes all rows, when run, then the CLI prints `no rows after filter` to stderr, exits
  `1` and writes no chart; the TUI shows an empty-state panel with the same message and stays interactive.
- Given sort and filter are both supplied, when run, then filtering is applied before sorting.
- Given invalid filter syntax or a missing column, then it prints the grammar/available columns and exits `2`.

**FR-9 — chart type selection.** The system must render line, bar, donut and pie, and pick a deterministic default
when the type is not given.

- Canonical `--type` values: `line` (2D plot), `bar` (graph), `pie`, `donut`. Accepted aliases: `plot` → `line`,
  `graph` → `bar`. Any other value is a usage error (`2`) listing the valid values.
- Default rule, documented in `--help`:
  - Exactly one value series, a categorical label, at most 30 rows and at most 30 distinct labels, and enough
    width for one bar per row → `bar`.
  - Otherwise → `line` (time/numeric x-axis, or ordinal x-axis for categorical data). If categorical data exceeds
    the bar threshold, one warning explains that row order is used and suggests `--type bar`/`--filter`.
  - `pie` and `donut` are never auto-selected.
- Compatibility matrix:
  - `line`: 1–8 series, any label type.
  - `bar`: exactly 1 series; values may be negative (zero baseline); rows must fit the available width.
  - `pie`/`donut`: exactly 1 series, 2–12 non-null rows, unique labels, all finite and non-negative, total > 0.
    Zero-valued slices are dropped with a note; negative, NaN, infinite values or duplicate labels are rejected
    (no implicit aggregation).
- Given an explicit type that is incompatible with the data (e.g. pie with 3 series, or bar with 2 series), then
  it prints the reason, suggests a compatible type and exits `2`.
- Given a valid explicit type, then it is used regardless of the default rule.
- Given an auto-selected type that cannot fit (e.g. bar categories exceed the width), then it falls back to line if
  possible; if no valid rendering exists, it exits `1` with guidance.
- No stacking, grouping, dual axes or log scales in v1.

### 6.3 Output

**FR-10 — CLI by default.** The system must render to stdout as one static chart and must not start the TUI unless
explicitly asked.

- Given no `--tui`, when run in any terminal, then exactly one static frame is written to stdout and the process
  exits `0`. No alternate screen, cursor movement, spinner or ticker is used.
- Given stdout is not a TTY, when run, then the output is plain text: no ANSI SGR, no escape sequences, no cursor
  control, only printable characters and newlines. `--color=always` is the explicit exception for tools such as
  `less -R`.
- Given `NO_COLOR` is set to a non-empty value, when run on a TTY, then color is disabled; hierarchy uses bold, dim,
  underline and spacing only. Given `TERM=dumb`, output is plain ASCII with no SGR at all.
- Given `--color=never`, color is disabled; given `--color=always`, SGR is emitted even off a TTY; `--color=auto`
  (default) colors only a TTY with color enabled. An explicit `--color` wins over `NO_COLOR`/`TERM`.
- Given `--ascii` or a non-UTF-8 locale/`TERM=dumb`, then all glyphs come from the ASCII set in §11.3; otherwise
  UTF-8 box drawing, braille and block elements are used.
- The frame is rendered fully in memory and written once. A write failure caused by a closed pipe terminates
  quietly with the platform's SIGPIPE behavior (exit `141` on Unix); no stack trace, no error banner.
- Output ends with exactly one newline, has no trailing spaces or tabs, and is byte-identical across repeated runs
  with the same inputs, options, width, locale, color and glyph modes (NFR-5).
- Default size: `80×24` when stdout is not a TTY; on a TTY, `min(terminal width, 100) × min(terminal height, 40)`.
  `--width` and `--height` override within 20–500 and 8–200 cells.
- Given a terminal narrower than 20 columns or shorter than 8 rows, then `pl` prints one line explaining the
  minimum size and exits `1`; it never draws a broken chart.

**FR-11 — TUI mode.** The system must provide an optional interactive mode with a framed chart and short
animations.

- `--tui` requires stdin and stdout to be TTYs. Given either is not a TTY (including piped input), then `pl` falls
  back to CLI mode with one warning on stderr and the normal CLI exit code. To explore piped data, save it to a
  file first.
- Given TUI mode on a TTY, then the alternate screen is used, the cursor is hidden while running, and the view has:
  a title/source header, a rounded chart frame, a status bar and a one-line help bar.
- Keys: `q`/`Ctrl+C` quit; `tab`/`shift+tab` cycle compatible chart types; arrows and `hjkl` scroll overflowing
  content (the full help panel; charts are fit to the viewport and do not scroll); `?` toggles full help; `esc`
  closes help. Every binding is a `key.Binding` with help text; no Alt/function keys are required. Mouse capture is
  not enabled in v1.
- Given a chart-type change, the new chart redraws in place with a highlight/fade of at most 150 ms; there is no
  sliding screen transition (DESIGN.md). First paint reveals marks over at most 400 ms at ≤30 fps.
- Reduced motion: `--no-motion`, `NO_COLOR`, `TERM=dumb`, or `CI` set ⇒ no animation; the first frame is static.
- Given a terminal resize, then the next frame re-lays out from the new `tea.WindowSizeMsg`; the minimum TUI size
  is 40×20. Below it, a centered "terminal too small" panel replaces the chart while `q`/`?` still work.
- Given the user quits or sends `Ctrl+C` after a successful load, then the terminal is restored (cursor visible,
  alternate screen left) and the exit code is `0`. If loading/rendering failed, any quit after the error panel is
  shown exits `1`; `Ctrl+C` while the initial load is still running exits `0`.
- Warnings (ragged rows, coerced nulls, ignored label columns) are shown in the status bar and full help, never
  printed to stderr while the screen is active. Errors show a friendly sentence, the cause and the next step.
- The TUI performs parsing, querying and cache writes in `tea.Cmd`s; `Update` and `View` perform no IO, no clock
  reads and no blocking work. Logs go only to the debug file.
- The same renderer and theme as CLI mode produce the chart, so the chart is identical modulo size and color mode.

**FR-12 — help and version.** The system must print usage, all flags, supported chart types, supported inputs and
the version without reading stdin.

- `pl --help` prints the synopsis, input modes, chart types, the default rule, the compatibility matrix, all flags,
  examples, exit codes and environment variables to stdout, exits `0`, and does not read stdin, load config or open
  a source.
- `pl --version` prints one line (`pl <version> (<commit>, <date>)`, or `pl dev` when ldflags are unset) to stdout,
  exits `0`, and does not read stdin or load config. The version comes from `-ldflags`, never a constant.
- Unknown flags or invalid values print one localized line plus `run 'pl --help'` to stderr and exit `2`. `--help`
  and `--version` are handled after flag parsing but before config is loaded or any source is opened, so they never
  wait for stdin; an unknown flag alongside them is still a usage error (`2`).
- Help and usage strings are message IDs in the i18n bundle (NFR-8); flag names remain English.

### 6.4 Cache

**FR-13 — cache.** The system must cache parsed datasets so that re-rendering the same input avoids re-parsing and
re-inference.

- The cache is a disposable SQLite database at `os.UserCacheDir()/pl/cache.db` (override with `--cache-dir` or
  `PL_CACHE_DIR`), created with directory mode `0700` and file mode `0600`. `--no-cache` / `PL_CACHE_DISABLE`
  disables reads and writes without deleting anything.
- The key is SHA-256 over: cache schema version, source kind, canonical source identity and parse options
  (delimiter, header mode). Source identity:
  - stdin: hash of the raw bytes;
  - file(s): absolute path, size, mtime (ns) and content hash, in argument order;
  - SQLite: absolute path, size, mtime (ns) and hash of the query text; the query text itself is not stored.
- A hit decodes a validated `Dataset` and skips parsing and inference. A hit updates `last_used_at` best-effort; a
  locked/busy cache skips the update rather than delaying the frame. A miss parses/queries, infers, renders, then
  writes the cache after the first frame; the write is best-effort and never delays or changes the chart.
- The read path never creates or migrates the cache database: an absent/uninitialized cache is a miss, and schema
  creation/migration runs on the write path after the first frame. Cache work never delays the first frame.
- Given the source content or parse options change, the key changes and the stale entry is not used.
- Given a corrupt entry, a schema-version mismatch or an unreadable payload, then the entry (or the whole cache DB)
  is discarded without a user-facing message (debug log only), the source is re-parsed, and the exit code/output
  are unaffected.
- Given the cache directory is unwritable or the cache is locked by another process, then the render still
  succeeds; cache errors are debug-level notes, never fatal and never a user-facing error.
- TTL is 7 days and the size cap is 64 MiB, evicting least-recently-used entries on write. An entry larger than the
  cap is not cached. Expired entries are treated as misses; physical cleanup is opportunistic on write, not on the
  render path.
- SQLite cache connections use WAL, `foreign_keys=ON` and a short `busy_timeout` (250 ms); on lock/busy, skip the
  cache rather than stall the frame. Schema migrations are embedded and applied once per schema version.
- A cache hit and a cache miss produce byte-identical output for the same inputs and options.

## 7. Non-functional requirements

**NFR-1 — Latency.** On a reference machine (4 vCPU x86_64, 16 GiB RAM, SSD, Linux, Go 1.25, 80×24, UTF-8, color),
for a 10,000-row × 5-column CSV, p95 from process start to first rendered frame is under 150 ms in CLI mode and
under 250 ms in TUI mode; a warm-cache CLI render is under 50 ms. Cold startup (`pl --version` or `--help`) is
under 100 ms, matching the STACK.md quality gate. Measure at least 20 runs after 3 warm-ups with an in-repo
benchmark/`hyperfine`; record the command and numbers in the milestone. The cache schema is created once; that
first-run cost is excluded from the warm number.

**NFR-2 — Memory.** Peak RSS stays under 100 MB for the 10,000-row dataset; the dataset is not duplicated in memory
more than twice (typed cells replace raw numeric strings, and the raw input buffer is released after parsing).
Cache decode follows the same bound. Measure with `/usr/bin/time -v` or `runtime.MemStats` in the benchmark.

**NFR-3 — Robustness.** No panic on malformed input; the CSV parser, filter parser and inference get fuzz tests.
Every failure is a message on stderr (CLI) or an error panel (TUI) plus a documented exit code. A broken stdout
pipe terminates quietly with standard SIGPIPE semantics, not a stack trace. Invalid UTF-8 is replaced with U+FFFD
and rendering continues. Data-derived text is terminal-safe: C0/C1 controls, ESC, Unicode line/paragraph separators
and bidi-format characters in column names, labels, titles and paths are removed or replaced with a space before
layout, so input cannot inject escape sequences; only `pl`'s own SGR is emitted. A top-level recover restores the
terminal, logs the stack to the debug file and exits `1` without printing the stack to the user.

**NFR-4 — Portability.** Single static Go binary, `CGO_ENABLED=0`, no runtime dependencies. Required targets:
linux/amd64, linux/arm64, darwin/amd64, darwin/arm64; also built by GoReleaser: windows/amd64 and windows/arm64
(ANSI requires Windows Terminal; legacy consoles degrade to plain text). Idiomatic Go: `gofmt`-clean,
`go vet`-clean, `golangci-lint` zero issues; no global mutable state in renderers.

**NFR-5 — Determinism.** Identical input, options, binary version, width/height, locale, timezone, color
mode/profile and glyph mode produce byte-identical CLI output. Sorting is stable with original row index as the final tie-break;
float formatting is fixed; no map iteration influences layout; series/color slots follow column or selection order.
Cache hit and miss do not change output. TUI frames at rest match their golden files after animation settles.

**NFR-6 — Accessibility of output.** Charts use Unicode box drawing, braille (U+2800–U+28FF) and block elements,
with an ASCII fallback selected by `--ascii`, a non-UTF-8 locale or `TERM=dumb`. Series are distinguishable by
marker shape and legend order as well as color; the palette is the Okabe-Ito set from DESIGN.md with an ANSI 16
fallback. Text never relies on red/green alone. `NO_COLOR`, `TERM=dumb` and non-TTY output are honored; no
background is painted; labels truncate with `…` (or `...` in ASCII) rather than wrapping. Check new screens with a
color-blindness simulator and in monochrome before calling them done.

**NFR-7 — Privacy.** No network egress at all in v1 (SQLite is local and the source DB is read-only); no telemetry.
The cache directory is `0700`; the cache DB, its WAL/SHM files and the debug log are created under umask `0077` so
all are user-only (`0600`). They contain only data the user already supplied plus derived types. Query text and row
values are never logged. Users can delete the cache or disable it. Future network databases must take credentials
from the environment/secret store, never from a committed config file.

**NFR-8 — Localization.** Every user-facing string (help, usage, errors, warnings, status and chart chrome) has a
stable message ID in `internal/i18n/active.en.toml`; translations are additive `active.<lng>.toml` files embedded
with `go:embed`. Locale resolution: `PL_LANG` > `LC_ALL` > `LC_MESSAGES` > `LANG` > `en`, via
`golang.org/x/text/language`; fall back to English and log a missing message, never panic. Numbers, row counts and
pie percentages use `x/text/message` for the active locale; time axes use unambiguous ISO-style labels in v1. Data
values are not translated. OS error details get a localized wrapper; database driver messages are emitted verbatim
with no wrapper (FR-4).

## 8. Data model

### 8.1 Entities

| Entity | Fields | Notes |
| --- | --- | --- |
| `Dataset` | `columns[]`, `rows[][]`, `source_kind` (stdin/file/db), `source_id`, `row_count`, `warnings[]` | In-memory table after parsing; rows are typed cells. |
| `Column` | `name`, `original_name`, `index`, `kind` (label/value), `value_type` (string/int/float/time/bool/empty/blob), `null_count`, `coerced_null_count`, `distinct_count` | `name` is unique after duplicate suffixing; `index` is 0-based internally, 1-based in flags. |
| `Cell` | `kind` (null/number/time/text/bool/blob), `number`, `time`, `text` | Numeric cells do not retain their raw string. |
| `Series` | `column`, `label`, `points[]`, `color_slot`, `marker` | One per plotted value column; `points` carry x (index/time) and y (number) with gaps for nulls. |
| `RenderSpec` | `chart_type`, `label_column`, `series[]`, `sort`, `filter`, `width`, `height`, `title`, `color_mode`, `glyph_mode`, `locale`, `timezone` | Resolved after inference and overrides; the sole input to the renderer. |
| `CacheEntry` | `key`, `schema_version`, `source_kind`, `source_id`, `created_at`, `last_used_at`, `size_bytes`, `payload` | One SQLite row; `payload` is the encoded, versioned `Dataset`. |
| `Warning` | `id`, `template_data` | Localized at the edge (CLI stderr or TUI status/help). |

### 8.2 Invariants

- A `Dataset` has 1..n columns; a header-only input has 0 rows and is still a valid parsed dataset.
- Column `name`s are unique; `original_name` may repeat. Duplicate headers are suffixed, never silently merged.
- A `RenderSpec` has exactly one label column and 1..8 series; chart-specific rules in FR-9 further constrain it.
  For a single numeric column, the label may be a synthetic `row` column (1..n) not present in `Dataset.columns`.
- `color_slot` is 0..7 assigned by stable series order; markers follow the same order.
- A `CacheEntry` key is unique; its payload decodes only under the matching `schema_version`.
- Data lives only in the process and the local cache; no dataset is shared between invocations in memory.

### 8.3 Cache schema

```sql
CREATE TABLE cache_entries (
  key            TEXT    PRIMARY KEY,
  schema_version INTEGER NOT NULL,
  source_kind    TEXT    NOT NULL,
  source_id      TEXT    NOT NULL,
  created_at     INTEGER NOT NULL,
  last_used_at   INTEGER NOT NULL,
  size_bytes     INTEGER NOT NULL,
  payload        BLOB    NOT NULL
) STRICT;

CREATE INDEX cache_entries_last_used_at ON cache_entries (last_used_at);
```

The schema lives in `db/migrations/` (goose), the queries in `db/queries/cache.sql` (`sqlc`), and the generated code
in `internal/db/` (committed, never hand-edited). `PRAGMA user_version` tracks the applied migration; a downgrade
or unknown version recreates the disposable cache.

## 9. Interfaces

### 9.1 Synopsis

```
pl [flags] [file...]
pl [flags] --db <sqlite-file> (--query <sql> | --query-file <path>)
pl --tui [flags] <file...>
pl --tui [flags] --db <sqlite-file> (--query <sql> | --query-file <path>)
pl --help | --version
```

`-` is a positional stdin source. At most one explicit source kind may be given: positional files (including `-`) or
`--db`. A piped stdin is ignored when a positional or database source is explicit. `--db` with a positional file,
`--query`/`--query-file` without any configured database path, and `--tui` with `--width`/`--height` are usage errors.

### 9.2 Flags

| Flag | Value | Default | Requirement |
| --- | --- | --- | --- |
| `--type` | `line`, `bar`, `pie`, `donut`; aliases `plot`, `graph` | auto (FR-9) | FR-9 |
| `--label` | column name or 1-based position | inferred | FR-5, FR-6 |
| `--columns` | list of names/positions; repeatable | all inferable value columns (max 8 for line) | FR-6 |
| `--rename` | `old=new`; repeatable | none | FR-7 |
| `--title` | text | `<source> — <type>` | FR-7 |
| `--sort` | `column[:asc\|desc]`; repeatable | source order | FR-8 |
| `--filter` | `column<op>value`; repeatable (AND) | none | FR-8 |
| `--delimiter` | `auto`, `comma`, `tab`, `semicolon`, `pipe`, a single rune | `auto` | FR-3 |
| `--header` / `--no-header` | boolean | auto-detect | FR-3 |
| `--db` | SQLite file path | config / `PL_DB` | FR-4 |
| `--query` | SQL text | config / `PL_QUERY` | FR-4 |
| `--query-file` | path to a SQL file | config `query_file` | FR-4 |
| `--timeout` | Go duration | `10s` | FR-4 |
| `--tui` | boolean | `false` | FR-11 |
| `--width` | 20–500 cells (CLI only) | terminal / 80 | FR-10 |
| `--height` | 8–200 cells (CLI only) | terminal / 24 | FR-10 |
| `--color` | `auto`, `always`, `never` | `auto` | FR-10 |
| `--ascii` | boolean | auto-detect | NFR-6 |
| `--no-motion` | boolean | `false` | FR-11 |
| `--no-cache` | boolean | `false` | FR-13 |
| `--cache-dir` | directory path | `os.UserCacheDir()/pl` | FR-13 |
| `--config` | TOML path | `os.UserConfigDir()/pl/config.toml` | §9.4 |
| `--help`, `-h` | boolean | — | FR-12 |
| `--version` | boolean | — | FR-12 |

Column names containing `,`, `=`, `:` or filter operator characters can be addressed by 1-based position instead.
For `--rename`, use repeated flags when a display label contains a comma.

Precedence: explicit flag > environment > config file > default. `--help` and `--version` are handled before config
is loaded or any source is opened, and never read stdin.

### 9.3 Exit codes

| Code | Meaning | Examples |
| --- | --- | --- |
| `0` | Success | Chart rendered; help/version; clean TUI quit. |
| `1` | Data or environment error | Empty stdin/file, header-only/no rows, parse error, no numeric series, no rows after filter, terminal too small, input too large, auto-render capacity, recovered internal panic. |
| `2` | Usage/configuration error | Unknown/invalid flag, no input, conflicting sources, missing DB parameter, missing/duplicate column, bad filter/sort/rename, explicit incompatible chart type, invalid config, TUI-only size conflict. |
| `3` | Source access error | Missing/unreadable file, mismatched multi-file headers, SQLite open/query/timeout failure (driver text verbatim). |
| `130` | Interrupted | SIGINT during a CLI parse or query. |
| `141` | Broken pipe | `pl ... \| head` on Unix; standard SIGPIPE behavior, no message. |

Warnings alone never change the exit code. Cache failures never change the exit code or output. In TUI mode a
successful session exits `0`; if the load failed and the user dismisses the error panel, it exits `1`.

### 9.4 Configuration

Default path: `os.UserConfigDir()/pl/config.toml` (Linux: `~/.config/pl/config.toml`), overridden by `--config` or
`PL_CONFIG`. A missing default file is fine; a missing explicitly requested file is a usage error (`2`). TOML is
parsed strictly: unknown keys are errors, not ignored. Paths in the config are resolved relative to the config
file's directory; paths from flags or environment are resolved relative to the current working directory.

```toml
[database]
path = "metrics.db"          # SQLite source; flags/env override
query = "select day, signups from metrics order by day"
# query_file = "queries/metrics.sql"   # mutually exclusive with query
timeout = "10s"

[display]
color = "auto"               # auto | always | never
ascii = false
width = 0                    # 0 = terminal; CLI only
height = 0                   # 0 = terminal; CLI only

[cache]
enabled = true
dir = ""                     # "" = os.UserCacheDir()/pl
ttl = "168h"
max_size = "64MiB"
```

### 9.5 Environment

| Variable | Effect |
| --- | --- |
| `PL_CONFIG` | Config file path. |
| `PL_DB` | SQLite source path. |
| `PL_QUERY` | SQL query text. |
| `PL_LANG` | Locale override (BCP 47). |
| `PL_CACHE_DIR` | Cache directory. |
| `PL_CACHE_DISABLE` | Non-empty disables cache reads/writes. |
| `PL_MAX_INPUT_BYTES` | Input cap; default `1073741824` (1 GiB). |
| `NO_COLOR` | Non-empty disables color (explicit `--color` wins). |
| `TERM` | `dumb` selects plain ASCII, no SGR, no animation. |
| `CI` | Non-empty disables TUI animation. |
| `DEBUG` | Non-empty enables `os.UserCacheDir()/pl/debug.log` (0600); no debug output on stdout/stderr. |
| `LANG`, `LC_ALL`, `LC_CTYPE`, `LC_MESSAGES` | Locale and UTF-8 detection. |
| `XDG_CACHE_HOME` | Base for the default cache directory on Unix. |

No database credentials are supported or stored in v1.

### 9.6 Examples

```sh
printf 'day,signups\nMon,3\nTue,5\nWed,4\n' | pl
psql -c 'select day, signups from metrics' --csv | pl
pl sales.csv
pl sales.csv --type bar --label month --columns revenue --sort revenue:desc --title 'Revenue by month'
pl sales.csv --filter 'region=EU' --filter 'revenue>=1000' --sort revenue:desc
pl sales.csv --rename revenue=Revenue --columns revenue,orders --type line
pl --db metrics.db --query 'select day, signups from metrics order by day' --type line
pl --tui sales.csv
NO_COLOR=1 pl sales.csv | less -R
pl --help
pl --version
```

## 10. Architecture

### 10.1 Layers and packages

| Package | Responsibility | May import |
| --- | --- | --- |
| `cmd/pl` | Flag/config parsing (cobra), source selection, composition root, exit-code mapping, CLI/TUI choice. | everything below |
| `internal/domain` | `Dataset`, `Column`, `Series`, `RenderSpec`, inference, filter/sort/selection rules, ports (`DatasetSource`, `CacheStore`). | standard library only |
| `internal/render` | Pure chart layout, axes, glyphs and ANSI output; no Bubble Tea, no Lip Gloss, no IO. | `domain`, `x/text`, display-width helper |
| `internal/storage` | stdin/file/SQLite source adapters; cache implementation over `internal/db`. | `domain`, `internal/db`, SQLite driver |
| `internal/db` | sqlc-generated cache queries (committed, never hand-edited). | generated code only |
| `internal/ui` | Bubble Tea root, keys, styles, commands, theme construction, screens. | `domain`, `render`, ports |
| `internal/ui/screens/plot` | `model.go`/`update.go`/`view.go` for the chart screen; no other screen imports. | `domain`, `render`, shared components |
| `internal/ui/components` | Reusable status bar/help/empty-state components. | `domain`, `render` |
| `internal/i18n` | Bundle, matcher, localized messages (`active.*.toml`, embedded). | `x/text` |
| `internal/testutil` | Builders, fixtures, fake clock/source/cache. | test-only |

Dependency direction: `cmd → ui → render → domain`, `cmd → storage → domain`, `ui → domain`; `domain` imports no UI,
Bubble Tea, SQL or driver package. Screens never import each other; the root routes messages. `internal/render` is
the one addition to the AGENTS/STACK layout (§15): it keeps CLI rendering out of Bubble Tea and avoids an import
cycle with `internal/ui/styles.go`.

### 10.2 Data flow

1. `cmd` parses flags; `--help`/`--version` short-circuit before config is loaded or any source is touched.
2. Exactly one source is selected; the cache key is computed from the source identity and parse options.
3. Cache lookup (unless disabled): a hit decodes a validated `Dataset`; a miss parses/queries, infers types and
   records warnings.
4. Domain transforms run in order: filter → sort → select → rename/title → build `RenderSpec`.
5. CLI mode renders `RenderSpec` to a buffer, writes the frame once, then writes the cache entry best-effort.
6. TUI mode builds the root model; loading, querying and cache writes run in `tea.Cmd`s; `View` renders from the
   model. Quit restores the terminal.
7. Errors are converted to localized messages and exit codes once, at the edge (`cmd` for CLI, the root model for
   TUI). Domain code returns typed/sentinel errors wrapped with `%w`; it never calls `os.Exit`.

### 10.3 CLI/TUI split

- CLI mode is sequential and does not create a Bubble Tea program; it may block on parse/query and renders exactly
  one frame.
- TUI mode owns stdin/stdout on the alternate screen. It requires a TTY and a file/DB source; piped stdin falls back
  to CLI because Bubble Tea needs stdin for key events.
- Both modes share `internal/render` and the theme constructed in `internal/ui/styles.go`; the chart must not be
  reimplemented per mode.
- All TUI side effects (read, query, cache write, tick) are `tea.Cmd`s returning typed messages; `Update`/`View` are
  pure. CLI main is allowed to block because it is not inside the UI loop.

### 10.4 Error mapping

- Domain errors are types/values (`ErrNoData`, `ErrNoNumericColumn`, `ErrColumnNotFound`, `ErrNoRowsAfterFilter`,
  `ErrIncompatibleType`, `ErrSource`, `ErrUsage`) declared in `internal/domain` and mapped once at the edge.
- File source errors wrap the OS text with localized context. Database driver messages are written to stderr
  exactly as returned; `pl` adds no wrapper, translation or rewriting. Missing DB parameters are localized usage
  errors.
- Cache errors are logged at debug level and swallowed; they never change user-visible behavior.
- No panics for control flow. A recovered panic restores the terminal (TUI), writes the stack only to the debug
  file, prints one line and exits `1`.

## 11. Output and interaction rules

### 11.1 CLI rendering

- Layout: one title line (`--title`, or `<source> — <type>` per FR-7), the chart body with y-axis labels and up to
  two x-axis lines, and a legend of one or two lines. No outer border in CLI mode; data area is prioritized.
- Line chart: braille subcells (2×4 per cell), 5 y ticks, sampled x labels, markers at sampled points for
  multi-series readability, gaps for nulls.
- Bar chart: vertical block-element bars from a zero baseline; positive bars up, negative bars down; one-cell gaps
  when width allows; sampled category labels; all-zero/constant data uses a stable mid-height guard.
- Pie/donut: compensate for the ~2:1 terminal cell aspect ratio so the shape is roughly circular; segments are
  colored, separated by visible gaps/edges and also listed in the legend with localized percentages
  (largest-remainder rounding, one decimal); donut has a hollow center. No text on slices.
- Axes/numbers: SI suffixes (`k`, `M`, `G`, `m`, `µ`, `n`), 3 significant digits, locale decimal separator, no
  `-0`. Time axes use the shortest unambiguous ISO-style label for the span. In ASCII mode `µ` is `u`, `×` is `x`,
  `—` is `-`, `…` is `...` and U+FFFD is `?`.
- Colors: UI chrome uses adaptive ANSI roles from `internal/ui/styles.go`; series use the Okabe-Ito palette in
  truecolor with an ANSI 16 fallback. No background is painted. Monochrome output uses marker shape and legend
  order.
- Warnings go to stderr, one line per category, deduplicated; stdout contains only the chart.
- If the renderer cannot fit the minimum, print one line and exit `1` (FR-10).

### 11.2 TUI rendering

- Frame: header (title, type badge, source, counts), rounded chart frame (focused `primary`, unfocused `muted`),
  status bar (source, rows×columns, series, nulls, warnings badge) and a one-line help bar. Content column is at
  most 100 cells, centered on wide terminals; no side-by-side panes below 60 columns.
- 80×24 is the design target; 40×20 is the usable minimum. Below 40×20, a centered minimum-size panel replaces the
  chart; `q`, `?` and resize still work.
- Help: short line by default, `?` opens the full `bubbles/help` view; every binding has help text and a single-key
  or Ctrl alternative. Keys from FR-11.
- Loading: spinner + verb (`Reading sales.csv…`); errors: `✗` + cause + next step; empty: `• No rows after filter`
  + the active filter. In ASCII mode use `x`, `*` and `...`. Use DESIGN.md components and tokens; no raw colors in
  views.
- Animation runs on a ticker owned by the UI loop and stops when the chart is settled. Reduced motion per FR-11.
- The view is laid out from the last `tea.WindowSizeMsg`; nothing is drawn from a hard-coded terminal size.
- Terminal restore on quit, `Ctrl+C`, error exit and recovered panic: cursor visible, alternate screen left,
  bracketed paste/mouse modes off.

### 11.3 Glyphs, color and series

| Chart | Unicode | ASCII fallback | Notes |
| --- | --- | --- | --- |
| Line | braille `U+2800–U+28FF`, box-drawing axes | `*`, `+`, `-`, `\|` | Markers at sampled points; gaps for nulls. |
| Bar | blocks `▁▂▃▄▅▆▇█` and `█` | `#`, `*`, `+` | Zero baseline; negative bars go down. |
| Pie/donut | braille/blocks + box-drawing frame | `#`, `*`, `+`, `.` | Legend carries labels and percentages. |
| Frame (TUI) | rounded `╭─╮│╰╯` | `+ - \|` | CLI has no outer frame. |

- Series palette order: Okabe-Ito orange, sky blue, bluish green, yellow, blue, vermillion, reddish purple, grey;
  ANSI 16 fallback follows DESIGN.md. Markers: `● ▲ ■ ◆ ▼ ○ ◇ ✚` with ASCII `o ^ # * v O + x`.
- Color is never the only signal: legend shows marker + label + (for pie) percentage; monochrome line charts place
  distinct markers at sampled points.
- No Nerd Font, glyph or color-depth requirement; detect and degrade. `--ascii` forces the fallback.
- Contrast: adaptive roles; muted text is used only for secondary information; important labels use the default
  foreground. Test light/dark themes and `NO_COLOR=1` per DESIGN.md.

### 11.4 Size and truncation

| Surface | Minimum | Target | Behavior below minimum |
| --- | --- | --- | --- |
| CLI | 20×8 | 80×24 | One-line message on stderr, exit `1`; no partial chart. |
| TUI | 40×20 | 80×24 | Centered panel; `q`/`?`/resize still work. |
| TUI content column | 40 | 80–100 | Center at ≤100; no side panes below 60; single column below 40. CLI `--width` may explicitly exceed 100. |

- All widths and truncation use terminal display width and grapheme clusters (wide CJK/emoji count as 2), never
  byte length or rune count.
- Labels shorten before data: title, then legend, then x labels; chart marks are the last to go.
- Truncate with `…` (Unicode) or `...` (ASCII); never wrap the chart off-screen. Counts/units shorten before names
  (`Downloads` → `DL`, `1,234,567` → `1.2M`).
- Long series labels do not widen the chart beyond the requested width.

## 12. Edge cases

**Input**

- Empty stdin: `no data on stdin` on stderr, exit `1`; do not hang.
- Empty file: `no data in <path>`, exit `1`. Header-only file: render a valid `0 rows` frame/notice, exit `1`.
- BOM, CRLF, quoted delimiters/newlines: parse correctly; line numbers in errors refer to the record start.
- Ragged rows: pad short rows with nulls, ignore extra fields, warn once with the first offending record number;
  render and exit `0` if the chart is valid.
- Non-UTF-8 or NUL bytes: replace with U+FFFD (`?` in ASCII mode), warn once, keep rendering; never abort.
- Labels/titles/paths containing ESC, C0/C1 controls, Unicode line/paragraph separators or bidi-format characters:
  remove or replace them with spaces before layout; never emit raw input escapes (NFR-3).
- Multiple files with mismatched headers/field counts: source error `3` naming the first mismatch.
- Input over `PL_MAX_INPUT_BYTES`: one-line size error, exit `1`.
- A path that is a directory/unreadable: source error `3`; a FIFO/pipe path is read normally.

**Values**

- All-null or all-text column where a value is required: exclude with a warning; if no series remain, list the
  available columns and exit `1`.
- Mixed numeric column (`N/A`, `-`, `1.2e3`): parse what parses, coerce the rest to null, state the null count.
- `NaN`/`Inf`/overflow: null for value columns; reject for pie/donut.
- Duplicate labels: suffix names, keep rows distinct, warn once; sorting/selection use the suffixed names.
- Extremely long labels: truncate with an ellipsis; never wrap or overflow.
- Single row or all-identical values: valid non-degenerate frame; no divide-by-zero.
- Pie/donut negative, zero-total, NaN or infinite: reject with a reason and a compatible-type suggestion (`2`).
- Pie/donut with duplicate labels: reject with a suggestion to filter or use bar (no implicit aggregation).
- Time columns with mixed/invalid formats: fall back to categorical labels with one warning; no crash.
- BLOB columns from SQLite: text label `<blob N bytes>`; selecting one as a value is a usage error (`2`).

**Flags and sources**

- No input (TTY stdin, no file/DB): usage on stderr, exit `2`.
- `--db` without a configured query, or `--query` without a configured database path: error names the missing
  parameter, exit `2`.
- `--db` plus a positional file: usage error `2`; `--query` plus `--query-file`: usage error `2`.
- A missing column referenced by `--label`, `--columns`, `--rename`, `--sort` or `--filter`: usage error `2` listing
  available names; renames are display-only.
- `--columns` selects the label column: usage error `2` suggesting `--label`.
- Empty `--columns`/`--label` values or malformed `--columns` lists: usage error `2`.
- `--header` and `--no-header` together: usage error `2`.
- `--width`/`--height` outside bounds or used with `--tui`: usage error `2`.
- Explicit incompatible chart type: usage error `2` with reason + suggestion (FR-9).
- Explicit `bar` whose rows cannot fit the width: rendering-capacity error `1`, with a suggestion to widen, filter
  or use `--type line`.
- Auto-selected chart cannot fit: fall back to line or exit `1` with guidance.
- `--tui` without a TTY, with piped stdin, or with stdout redirected: CLI fallback + one stderr warning; normal
  exit code.
- `--help`/`--version` with piped stdin: exit `0`, input not consumed; both are handled before config is loaded or
  any source is opened.

**Terminal and process**

- Terminal narrower than 20 or shorter than 8 (CLI): one-line message, exit `1`; TUI: minimum panel, `q` works.
- Broken pipe on stdout: quiet SIGPIPE, exit `141` on Unix; no stack trace.
- Ctrl+C during a CLI parse/query: cancel, exit `130`; in TUI: restore; exit `0` during a successful session or
  while the initial load is running, and `1` after a load error panel.
- `NO_COLOR`, `TERM=dumb`, non-TTY, ASCII locale: plain/monochrome output remains readable; no SGR off a TTY
  unless `--color=always`.
- `COLORTERM`/truecolor unsupported: ANSI 16 series fallback; no broken escapes.
- Recovered panic: terminal restored, stack only in the debug file, one-line user message, exit `1`.

**Cache and database**

- Corrupt cache DB/entry, schema mismatch, payload decode failure: discard without a user-facing message (debug log
  only) and re-parse; output and exit code unaffected.
- Unwritable/locked cache: skip cache, render normally; never fatal, never a user-facing error.
- Cache entry over the size cap: do not cache; evict LRU entries on write; TTL 7 days.
- SQLite missing/non-SQLite/locked/read-only-write attempt: driver message verbatim, exit `3`.
- Query timeout: clear timeout message, exit `3`; Ctrl+C during the query follows FR-11 (TUI) or exits `130` (CLI).
- Query returns zero rows: header-only behavior (`0 rows` notice, exit `1`).

## 13. Decisions (resolved open questions)

The previous draft's Q1–Q16 are resolved as follows. Later changes need a spec edit.

| # | Decision |
| --- | --- |
| 1 | No input (TTY stdin, no source) → usage on stderr, exit `2`; never read the terminal. |
| 2 | Auto-detect comma, tab, semicolon and pipe; `--delimiter` overrides with a rune or name. JSON/JSONL are out; TSV is tab-delimited. |
| 3 | v1 reads SQLite only, read-only, via `--db` + `--query`/`--query-file` or config/env. PostgreSQL/MySQL are served by piping `psql --csv` or `mysql --batch` output. |
| 4 | Final flags are the table in §9.2; `--type` uses `line`/`bar`/`pie`/`donut` with `plot`/`graph` aliases. |
| 5 | Data-driven default: one series + categorical label (≤30 rows/labels that fit) → bar; otherwise line. Pie/donut are explicit only. |
| 6 | Pie/donut accept exactly one series, 2–12 unique non-negative categories, total > 0; extra series are rejected with a suggestion, not summed. |
| 7 | `--tui` without a TTY falls back to CLI with one stderr warning; it does not hard-error. |
| 8 | Cache key is a content/identity hash plus parse options and schema version; SQLite cache at `os.UserCacheDir()/pl/cache.db`, TTL 7 days, cap 64 MiB, `--no-cache` available. |
| 9 | NFR-1 targets stay: 150 ms CLI, 250 ms TUI, 50 ms warm; reference hardware and measurement are defined there. |
| 10 | Binary is `pl`; GoReleaser produces archives/checksums/Homebrew tap; no separate `plot` binary in v1. |
| 11 | Streaming/live input is out of scope; `pl` reads a finite snapshot and exits. |
| 12 | Exit codes are the table in §9.3: `0`, `1`, `2`, `3`, `130`, `141`; cache failures never change them. |
| 13 | Ragged rows are padded/truncated and warned once; they do not abort the run. |
| 14 | Filters are simple `column op value` expressions ANDed together, available for all sources; no SQL/OR/grouping. |
| 15 | "Graph" means bar; v1 has four types: line, bar, pie, donut. |
| 16 | Renames are display-only; `--columns`, `--label`, `--sort` and `--filter` always use original parsed names. |

## 14. Deferred

- Native PostgreSQL/MySQL sources (use the client pipe).
- Multi-series bar, grouped/stacked bars, multi-series pie/donut.
- Horizontal bars for very long labels or many categories.
- JSON/JSONL/Parquet/Excel input, compression, encoding negotiation beyond UTF-8 replacement.
- Streaming/`tail -f` input and live-updating TUI.
- Log scales, dual axes, trendlines, regression, aggregation, pivots.
- Heatmaps, maps, 3D, scatter matrices.
- Image/SVG/HTML/PDF export.
- TUI zoom/pan/row inspector beyond scrolling, mouse capture, saved sessions.
- Custom palettes/themes, additional locales (English is the source and required fallback), shell completions.
- Windows legacy-console color support (Windows Terminal is the supported ANSI path).

## 15. Stack alignment

The spec follows STACK.md, DESIGN.md and AGENTS.md. These resolutions are reflected in those documents:

- v1 reads SQLite only, read-only and CGO-free through `modernc.org/sqlite`; PostgreSQL and MySQL are served by
  `psql --csv` / `mysql --batch` pipes. There is no `pgx` dependency.
- The only persistent store `pl` writes is the disposable cache at `os.UserCacheDir()/pl/cache.db`; source
  databases are never modified. `sqlc` generates typed cache queries, `goose` authors the migrations, and the app
  applies the same embedded statements with `PRAGMA user_version` at runtime.
- `internal/render/` is a pure chart package: no Bubble Tea and no Lip Gloss. `internal/domain/` stays plain Go.
- The TUI is a single screen in `internal/ui`; it splits into `screens/plot/` only when a second screen appears.
- The locale comes from `PL_LANG` > `LC_ALL` > `LC_MESSAGES` > `LANG` > `en`; there is no HTTP request or per-request
  localizer.
- Commands build `bin/pl` or `pl` with `CGO_ENABLED=0`; the documented exit codes are §9.3.

DESIGN.md needs no rule changes: this spec uses its adaptive ANSI roles, Okabe-Ito palette, braille/block glyphs,
ASCII fallback, 80×24 target, 40×20 minimum, TUI 100-cell content column, rounded TUI frame, no background
painting, and reduced-motion rules. The added runtime dependencies are the pure-Go SQLite driver, TOML parser and
`golang.org/x/text` (already present for i18n).

## 16. Milestones

1. **Pipe to chart.** Repo scaffold per AGENTS; domain types and inference; RFC 4180 parser with delimiter/header
   detection; line and bar renderers; CLI stdin path; help/version; exit codes; no-TTY and broken-pipe handling.
   Ships FR-1, FR-3 (comma+header), FR-5, FR-10, FR-12; NFR-3/5/6.
2. **Four chart types.** Pie/donut, default rule and compatibility matrix, series palette/markers, ASCII/`NO_COLOR`
   goldens, size/truncation rules. Ships FR-9; NFR-6.
3. **Files and transforms.** Multiple files, `--label`/`--columns`, `--rename`/`--title`, `--sort`/`--filter`,
   empty-result behavior, missing-column diagnostics, CLI contract tests. Ships FR-2, FR-6, FR-7, FR-8.
4. **TUI.** Bubble Tea app, styles/keys/help, alt screen, chart frame, first-paint animation, reduced motion,
   resize, TTY fallback, terminal restore, teatest goldens. Ships FR-11.
5. **SQLite source.** Read-only adapter, config/env resolution, query timeout, verbatim driver errors, integration
   tests with a temp database. Ships FR-4.
6. **Cache and hardening.** Goose/sqlc cache schema, key/TTL/LRU, cold/warm/corrupt/locked tests, 10k benchmark,
   memory measurement, fuzz targets, cross-compile and release dry run. Ships FR-13, NFR-1/2/4/7.

## 17. Verification and definition of done

| Area | Required evidence |
| --- | --- |
| Domain | Table-driven unit tests for inference, null handling, selection, filter grammar, sort stability, rename/title, chart compatibility; fuzz tests for CSV/filter/inference. |
| Renderer | Golden files at 80×24, 40×20, 120×40 and 200×60 for line/bar/pie/donut; color, `NO_COLOR`, ASCII; nulls, negatives, single row, all-equal, long labels; CLI and TUI. |
| CLI contract | Compiled-binary tests for stdin/file/DB, `--help`/`--version` not reading stdin, exit codes `0/1/2/3/130/141`, non-TTY has no ESC/SGR, `--color=always` exception, broken pipe, SIGINT. |
| TUI | `teatest` at 80×24 and 40×20: first frame, `tab` type cycle, `?` help, resize, quit, reduced motion, `NO_COLOR`/ASCII, load error panel. |
| Storage | Temp SQLite with migrations; read-only source; missing/non-SQLite/locked driver errors verbatim; timeout message; cache cold/warm/corrupt/locked/unwritable; `0600`/`0700` permissions; hit/miss output identical. |
| Performance | Benchmarks for 10k×5 cold CLI, warm CLI and TUI first frame; p95 against NFR-1; RSS against NFR-2; numbers recorded in the milestone. |
| Determinism | Render the same spec 100 times and compare bytes; run with cache on/off and compare; stable-sort tie cases. |
| i18n | A test loads every `active.*.toml`, checks every message ID/template used by the code, and verifies fallback to English. |
| Quality gates | `go build ./...`, `gofmt -l -w .`, `go vet ./...`, `golangci-lint run`, `go test ./...`, `go test -race ./...`, goldens committed, `sqlc generate` leaves no diff, `goreleaser check`. |

Definition of done (in addition to the AGENTS checklist):

- [ ] Every FR in §6 has at least one automated test, and every NFR has a measurement or check.
- [ ] `pl --help` matches §9 and is generated from the i18n bundle; flags/exit codes were not changed silently.
- [ ] CLI contract tests pass on a compiled binary, including non-TTY output and broken pipe.
- [ ] `internal/domain` imports no UI, Bubble Tea, Lip Gloss, SQL or driver package; `internal/render` imports no
      Bubble Tea.
- [ ] Cache failure paths are exercised and never change output or exit code.
- [ ] STACK/AGENTS updates from §15 are applied, or the spec is amended before implementation.
- [ ] Diff is minimal, no debug output on stdout, no secrets, no generated files hand-edited.
