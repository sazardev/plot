#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
EXAMPLES="$ROOT/examples"
BIN="${PL_BIN:-$ROOT/pl}"

menu() {
  cat <<'EOF'
pl example workspace

  1  bar         Revenue bar chart from CSV
  2  line        Multi-series line chart from CSV
  3  donut       Donut and pie charts from categories
  4  pipe        CSV piped through stdin
  5  transforms  Filter, sort, rename and title
  6  sqlite      Read-only SQLite query
  7  config      Database + display through TOML config
  8  tui         Interactive TUI (requires a real terminal)
  9  delimiters  Semicolon, tab and headerless input
 10  edge        Nulls, negatives, duplicates and ragged rows
 11  errors      Expected failures and exit codes
 12  cache       Cold/warm cache demonstration
 13  web         Files for the browser playground
 14  all         Run the main chart scenarios
  q  quit

Usage: ./examples/run.sh <number|name>
EOF
}

heading() {
  printf '\n\033[1m== %s ==\033[0m\n\n' "$1"
}

expect() {
  local label="$1" expected="$2"
  shift 2
  heading "$label (expected exit $expected)"
  set +e
  "$@"
  local code=$?
  set -e
  printf '\nexit=%s (expected %s)\n' "$code" "$expected"
}

ensure_bin() {
  if [[ -x "$BIN" ]]; then
    return
  fi
  printf 'Building pl into %s...\n' "$BIN"
  (cd "$ROOT" && CGO_ENABLED=0 go build -o "$BIN" ./cmd/pl)
}

scenario="${1:-}"
if [[ -z "$scenario" ]]; then
  menu
  printf '\nChoose a scenario: '
  read -r scenario
fi

case "$scenario" in
  1|bar)
    ensure_bin
    heading "Bar chart · revenue by month"
    "$BIN" "$EXAMPLES/revenue.csv" --type bar --title "Revenue by month"
    ;;
  2|line)
    ensure_bin
    heading "Line chart · traffic by source"
    "$BIN" "$EXAMPLES/traffic.csv" --type line --title "Traffic by source"
    ;;
  3|donut|pie)
    ensure_bin
    heading "Donut · sessions by channel"
    "$BIN" "$EXAMPLES/channels.csv" --type donut --title "Sessions by channel"
    heading "Pie · sessions by channel"
    "$BIN" "$EXAMPLES/channels.csv" --type pie --title "Sessions by channel"
    ;;
  4|pipe)
    ensure_bin
    heading "CSV through stdin"
    cat "$EXAMPLES/orders.csv" | "$BIN" --type bar --label region --columns revenue \
      --sort revenue:desc --title "Revenue by region"
    ;;
  5|transforms)
    ensure_bin
    heading "Filter, sort, rename and title"
    "$BIN" "$EXAMPLES/orders.csv" --filter "region=EU" --sort revenue:desc \
      --rename revenue=Revenue --label month --columns revenue \
      --title "EU revenue by month"
    heading "Multiple files? This workspace uses one for clarity."
    "$BIN" "$EXAMPLES/orders.csv" --columns revenue,cost --type line \
      --title "Revenue and cost by row"
    ;;
  6|sqlite)
    ensure_bin
    heading "SQLite · inline query"
    "$BIN" --db "$EXAMPLES/metrics.db" \
      --query "select day, signups, revenue from metrics order by day" \
      --type line --title "Signups and revenue"
    heading "SQLite · query file"
    "$BIN" --db "$EXAMPLES/metrics.db" --query-file "$EXAMPLES/queries/top-products.sql" \
      --type bar --title "Top products by revenue"
    ;;
  7|config)
    ensure_bin
    heading "TOML config · database, display and cache"
    "$BIN" --config "$EXAMPLES/config.toml"
    printf '\nConfig path: %s\n' "$EXAMPLES/config.toml"
    ;;
  8|tui)
    ensure_bin
    if [[ ! -t 0 || ! -t 1 ]]; then
      echo "The TUI needs stdin and stdout to be real terminals." >&2
      exit 1
    fi
    "$BIN" --tui "$EXAMPLES/channels.csv" --title "Sessions by channel"
    ;;
  9|delimiters)
    ensure_bin
    heading "Semicolon delimiter"
    "$BIN" "$EXAMPLES/edge/semicolon.csv" --delimiter semicolon --type line
    heading "Tab delimiter"
    "$BIN" "$EXAMPLES/edge/tabs.tsv" --delimiter tab --type line
    heading "Headerless input"
    "$BIN" "$EXAMPLES/edge/no-header.csv" --no-header --label 1 --columns 2 --type bar
    ;;
  10|edge)
    ensure_bin
    heading "Negative values · bar baseline"
    "$BIN" "$EXAMPLES/edge/negative.csv" --type bar --title "Monthly profit"
    heading "Nulls · line gaps"
    "$BIN" "$EXAMPLES/edge/nulls.csv" --type line --title "Null handling"
    heading "Duplicate headers · unique suffixes"
    "$BIN" "$EXAMPLES/edge/duplicate-headers.csv" --type line
    heading "Ragged rows · warning on stderr"
    "$BIN" "$EXAMPLES/edge/ragged.csv" --type line
    heading "Mixed number/text column"
    "$BIN" "$EXAMPLES/edge/mixed-types.csv" --type line
    ;;
  11|errors)
    ensure_bin
    expect "Empty file" 1 "$BIN" "$EXAMPLES/edge/empty.csv"
    expect "Header only" 1 "$BIN" "$EXAMPLES/edge/header-only.csv"
    expect "Missing file" 3 "$BIN" "$EXAMPLES/does-not-exist.csv"
    expect "Bad filter" 2 "$BIN" "$EXAMPLES/revenue.csv" --filter "revenue>>10"
    expect "Unknown chart type" 2 "$BIN" "$EXAMPLES/revenue.csv" --type candle
    printf '\nThese failures are intentional; stdout stays chart-only.\n'
    ;;
  12|cache)
    ensure_bin
    cache_dir="$(mktemp -d)"
    heading "Cold cache"
    XDG_CACHE_HOME="$cache_dir" "$BIN" "$EXAMPLES/revenue.csv" --type bar >/dev/null
    du -h "$cache_dir/pl/cache.db" 2>/dev/null || true
    heading "Warm cache"
    XDG_CACHE_HOME="$cache_dir" "$BIN" "$EXAMPLES/revenue.csv" --type bar >/dev/null
    printf 'Cache directory: %s\n' "$cache_dir/pl"
    printf 'The chart output is identical; cache work never changes it.\n'
    ;;
  13|export)
    ensure_bin
    export_dir="$(mktemp -d)"
    heading "Native export · CLI"
    "$BIN" "$EXAMPLES/revenue.csv" --type bar --title "Revenue by month" \
      --export "$export_dir/revenue.png" >/dev/null
    "$BIN" "$EXAMPLES/revenue.csv" --type bar --title "Revenue by month" \
      --export "$export_dir/revenue.jpg" >/dev/null
    "$BIN" "$EXAMPLES/revenue.csv" --type bar --title "Revenue by month" \
      --export "$export_dir/revenue.pdf" >/dev/null
    ls -lh "$export_dir"
    printf 'Exported files: %s\n' "$export_dir"
    printf 'TUI export: ./examples/run.sh tui, then e → p (PNG), j (JPG) or d (PDF).\n'
    ;;
  14|web|formats)
    heading "Browser playground files"
    cat <<EOF
Open: https://sazardev.github.io/plot/#playground

Upload or paste:
  $EXAMPLES/revenue.csv
  $EXAMPLES/formats/table.md
  $EXAMPLES/formats/data.json
  $EXAMPLES/formats/dump.sql
  $EXAMPLES/formats/report.xlsx   (two sheets)
  $EXAMPLES/metrics.db            (SQLite)

Try the local agent: Agent → Audit → Fix data, then Export PNG/PDF and Save plot.

Serve the site locally:
  python3 -m http.server -d "$ROOT/docs" 8765
  open http://localhost:8765/
EOF
    ;;
  15|all)
    "$0" bar
    "$0" line
    "$0" donut
    "$0" sqlite
    ;;
  q|quit|exit)
    exit 0
    ;;
  -h|--help|help|list)
    menu
    ;;
  *)
    echo "Unknown scenario: $scenario" >&2
    menu >&2
    exit 2
    ;;
esac
