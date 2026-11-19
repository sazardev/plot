# pl example workspace

A small, self-contained playground for trying `pl` with different inputs, chart types, transforms, SQLite,
edge cases and the browser-only formats. Everything here is safe to edit or delete.

## Start here

From the repository root:

```sh
./examples/run.sh
```

You get a menu. You can also run a scenario directly:

```sh
./examples/run.sh bar
./examples/run.sh sqlite
./examples/run.sh tui
```

The runner builds `./pl` with `CGO_ENABLED=0 go build -o pl ./cmd/pl` the first time, so Go 1.26+ is the only
requirement. Use an existing binary instead:

```sh
PL_BIN=/path/to/pl ./examples/run.sh line
```

## Scenarios

| # | Name | What it shows |
| --- | --- | --- |
| 1 | `bar` | Revenue bar chart from a CSV, custom title |
| 2 | `line` | Multi-series line chart with braille rendering |
| 3 | `donut` | Donut and pie charts with percentages |
| 4 | `pipe` | CSV through stdin, `--label`, `--columns`, `--sort` |
| 5 | `transforms` | `--filter`, `--sort`, `--rename`, multiple series |
| 6 | `sqlite` | Read-only SQLite with inline query and query file |
| 7 | `config` | TOML config for database, display and cache |
| 8 | `tui` | Interactive TUI; `tab`, `?`, `q` |
| 9 | `delimiters` | Semicolon, tab and headerless CSV |
| 10 | `edge` | Nulls, negatives, duplicate headers, ragged rows, mixed types |
| 11 | `errors` | Expected failures and exit codes |
| 12 | `cache` | Cold vs warm cache using a temporary cache directory |
| 13 | `web` | Paths and instructions for the browser playground |
| 14 | `all` | Run the main chart scenarios back to back |

## Files

```text
examples/
  revenue.csv             12 months of revenue; bar chart
  traffic.csv             7 days × 3 traffic sources; line chart
  channels.csv            5 channels; pie/donut input
  orders.csv              region/month/revenue/cost/channel; transforms
  metrics.db              SQLite source with metrics and products
  config.toml             database + display + cache config
  queries/
    weekly.sql            inline metrics query
    top-products.sql      query file scenario
  edge/
    negative.csv          positive and negative values
    nulls.csv             empty and N/A cells
    no-header.csv         headerless input
    semicolon.csv         semicolon-delimited input
    tabs.tsv              tab-delimited input
    ragged.csv            short and long rows
    duplicate-headers.csv duplicate header names
    mixed-types.csv       mixed number/text column
    empty.csv             empty input; exit 1
    header-only.csv       header without rows; exit 1
  formats/
    table.md              Markdown table for the browser playground
    data.json             JSON array of objects
    dump.sql              SQLite-compatible SQL dump
    report.xlsx           two-sheet Excel workbook
```

The `.db` and `.xlsx` files are committed fixtures. Regenerate the database with `sqlite3 metrics.db < ...`
if you want to change it.

## Browser playground

The CLI reads delimited text and read-only SQLite. The website additionally handles Markdown, JSON, SQL dumps,
Excel and pasted clipboard data, renders the chart in the browser and can export PNG/JPG/WebP/PDF or save local
history:

<https://sazardev.github.io/plot/#playground>

Run it locally:

```sh
python3 -m http.server -d docs 8765
```

Then open <http://localhost:8765/> and upload files from `examples/formats/`, `examples/metrics.db` or
`examples/revenue.csv`. The playground also has a local data agent with Status, Audit, Fix data, Recommend,
Guide, Export and Help skills.

## Edge cases and expected failures

The `errors` scenario intentionally fails:

```text
empty.csv        exit 1
header-only.csv  exit 1
missing file     exit 3
bad filter       exit 2
unknown chart    exit 2
```

`edge` is expected to succeed, with warnings on stderr for duplicate headers, ragged rows and multiple label
columns. Try `--ascii`, `--color=never`, `--width 40 --height 16`, or `--no-cache` by appending flags to any
command.

## Useful commands

```sh
# build once
CGO_ENABLED=0 go build -o pl ./cmd/pl

# use any of these files directly
./pl examples/revenue.csv
./pl examples/traffic.csv --type line
./pl examples/metrics.db --query 'select * from metrics'
./pl --config examples/config.toml
./pl --tui examples/traffic.csv
```

From the repository root, `pl --help` is the complete flag reference. The website guide is at
<https://sazardev.github.io/plot/#docs>.
