# DESIGN.md: Modern TUI (Charm-style)

The modern terminal look popularized by Charm (Glow, Gum, Soft Serve): rounded borders, soft adaptive colors that suit light
and dark terminals, generous padding, gentle spinners and markdown rendered with care. Friendly, polished, still fast.

## Principles

- **Friendly, not loud**: soft colors, rounded corners, breathing room.
- **Adaptive**: every color has a light and a dark variant chosen from the terminal background.
- **Guided**: inline help, clear prompts and forms that tell you what to do next.
- **Polish in motion**: spinners and small transitions show life without slowing anything.
- **Readable docs**: markdown is rendered (headings, code, lists), not dumped.
- **The terminal decides**: the app looks native in every theme the user already chose (Gruvbox, Catppuccin, Solarized…), light or dark, because it never paints its own background.

## Color

| Token | Light terminal | Dark terminal | ANSI 16 fallback | Use |
| --- | --- | --- | --- | --- |
| `fg` | `#1A1A1A` | `#DDDDDD` | default | Text |
| `muted` | `#8A8A8A` | `#6C6C6C` | 8 | Hints, help |
| `primary` | `#7D56F4` | `#9B7CFF` (approx.) | 5 | Titles, focus, selection |
| `secondary` | `#EE6FF8` | `#F78CFF` (approx.) | 13 | Highlights, badges |
| `success` | `#02BA84` | `#04D99A` (approx.) | 2 | Done |
| `warning` | `#C99700` (approx.) | `#F2C94C` (approx.) | 3 | Attention |
| `danger` | `#E0245E` (approx.) | `#FF5F87` | 1 | Errors |
| `surface` | `#F2F0FA` (approx.) | `#2A2438` (approx.) | none | Badges and selected items background |

- Adaptive colors via `lipgloss.AdaptiveColor{Light, Dark}`; background detected once at start.
- Colors are **ANSI roles, not hex**: default foreground/background for text, `8`/bright black for muted, `1` red for danger, `2` green for success, `3` yellow for warning, `4` blue for accent, `5` magenta for highlights, `6` cyan for links/info.
- Never set a background on the whole screen; selections use reverse video (`SGR 7`) or the accent as background with the default background as text.
- Truecolor (`COLORTERM=truecolor`) only for gradients and charts, always with an ANSI 16 fallback; 256-color in between.
- `NO_COLOR` set or `TERM=dumb` → no colors at all, hierarchy by bold, dim, underline and spacing alone; not a TTY → plain text.
- Success/danger pairs never rely on red vs green alone: add icons (`✓`, `✕`), labels or patterns; charts use a color-blind safe set (e.g. Okabe-Ito: `#E69F00`, `#56B4E9`, `#009E73`, `#F0E442`, `#0072B2`, `#D55E00`, `#CC79A7`).

## Typography

| Role | Style | Use |
| --- | --- | --- |
| Title | bold `primary`, often in a `surface` badge with 1-cell padding | Screen titles |
| Body | default | Text |
| Hint | `muted` | Inline help, placeholders |
| Key | bold | Shortcuts in the help view (`↑/k up • q quit`) |
| Markdown | Glamour `dark`/`light`/`auto` style | Docs, READMEs, long text |
| Icons | `✓ ✗ • → ●` (Unicode), Nerd Font icons optional | Status |

- Borders with box drawing: light `┌─┐│└┘`, rounded `╭─╮│╰╯`, heavy `┏━┓┃┗┛` for focus, double `╔═╗║╚╝` for dialogs.
- Bars and meters with block elements `▁▂▃▄▅▆▇█` (vertical) and `▏▎▍▌▋▊▉█` (horizontal); sparklines with the vertical set.
- Charts and fine graphics with braille (U+2800-U+28FF): 2×4 dots per cell, 8× the resolution of plain cells.

## Spacing and layout

- Unit = one cell; padding 1×2 (rows×cols) inside boxes; margins of 1 row between blocks.
- Minimum 80×24; centered content column of ≤ 100 cells on wide terminals; lists fill height.
- Help view at the bottom, one line short help, `?` toggles the full help.
- Usable from 40×20 cells: a single column, no side-by-side panes below 60 columns.
- Labels shorten before content does (`Downloads` → `DL`); tables drop low-priority columns first, by a declared priority.
- Designed for 80×24 cells and usable down to it; above 120 columns a second pane may appear.
- Redraw on every resize (`SIGWINCH`); nothing is cut silently: truncate with `…` and keep the key column readable.
- Runs the same over SSH, in tmux/screen and in the alternate screen; restores the terminal on exit, crash or `Ctrl+C`.
- GUI: 4 px base unit, rows 28-32 px, controls 28 px high, body text 13-14 px; TUI: no blank line between list rows, 1-cell padding inside borders.
- Information density over whitespace; group with alignment and subtle dividers instead of space.

## Shape and elevation

| Token | Glyphs | Use |
| --- | --- | --- |
| border-rounded | `╭─╮│╰╯` | Cards, panes, dialogs |
| border-thick-left | `▌` or `┃` in `primary` | Selected list item marker |
| badge | text on `surface` with 1-cell padding | Titles, tags |

- Elevation: focused pane gets a `primary` border; others `muted`. Dialogs are rounded boxes placed in the center (`lipgloss.Place`).

## Motion

| Token | Value | Use |
| --- | --- | --- |
| spinner | `spinner.Dot` / `spinner.MiniDot` (~10–12 fps) | Activity |
| progress | animated bar with spring easing (`bubbles/progress`), gradient `primary` → `secondary` | Determinate work |
| frame-budget | 30 fps | Animated components |
| transitions | none between screens | Instant |

- Reduced motion: progress bars jump to the value; spinners static.
- Spinner frames: braille `⠋ ⠙ ⠹ ⠸ ⠼ ⠴ ⠦ ⠧ ⠇ ⠏` at 80 ms/frame (dots), or `◐ ◓ ◑ ◒`; ASCII fallback `| / - \`.
- Progress bars with sub-cell precision (`▏▎▍▌▋▊▉█`), percentage and ETA; indeterminate work uses a spinner, never a fake bar.
- Typewriter or fade-in reveals only for short headings (≤ 40 chars, ≤ 400 ms total).
- Redraw budget 30 fps for idle animation, 60 fps max while interacting; only changed cells are redrawn.

## Components

- **Lists**: `bubbles/list` with title, filter (`/`), pagination dots, selected item with a `▌` marker and `primary` text.
- **Tables**: `bubbles/table`, header bold with bottom border, selected row in `surface`.
- **Forms**: `huh` forms: one question at a time or grouped, validation inline in `danger`.
- **Text input / text area**: placeholder in `muted`, focused prompt in `primary`.
- **Viewport**: scrollable markdown rendered by Glamour.
- **Status bar**: segments as colored badges (mode, file, position).
- **Help**: `bubbles/help` short and full views.
- **Empty/loading/error**: friendly sentence + icon (`• Nothing yet — press n to create one`), spinner + verb, `✗` + cause + next step.
- Every key binding has a single-key or `Ctrl` alternative (soft keyboards lack `Alt`/function keys); touch scrolling works where the terminal sends mouse events.
- Keyboard first: arrows and `hjkl`, `Enter` select, `Esc` back, `q` quit, `/` search, `?` help; the mouse is an optional extra.
- A status/help line shows the keys that matter on the current screen.

## Accessibility

- Adaptive colors keep contrast on light and dark terminals; check `muted` stays ≥ 4.5:1 for anything important (approx.).
- `NO_COLOR`/`TERM=dumb`: Lip Gloss drops colors; borders and markers still show focus.
- Non-TTY: print plain output (Gum-style: results to stdout, UI to stderr).
- Keyboard: arrows and vi keys, `/` filter, `?` help, `esc` back, `q`/`ctrl+c` quit.
- No animation when stdout is not a TTY, in `CI`, with `NO_COLOR`/`TERM=dumb`, or with the app's reduced-motion flag: print a single status line instead.
- Every screen is checked with a color-blindness simulator (deuteranopia, protanopia, tritanopia) before it is done.
- With `prefers-reduced-motion: reduce` (or the OS/app setting), movement is replaced by ≤ 150 ms opacity fades; parallax, auto-playing animation and spinners that move across the screen are removed.

## Rules for agents

- Use Bubble Tea + Bubbles + Lip Gloss + Huh + Glamour; Ratatui equivalent: rounded `BorderType::Rounded`, `tui-input`, `tui-markdown` (approx.).
- Styles only in `styles.go` with `AdaptiveColor`; never a raw color in a view.
- Every screen has a short help line; every action is in the full help.
- Respect the terminal background: no full-screen background painting.
- Test with light and dark terminal themes, `NO_COLOR=1` and at 80×24.
- Test at 40×20 and at 60×24 as well as 80×24.
- Test at 80×24, at 200×60 and while resizing; test inside tmux and over SSH.
- Never assume a font, a color depth or mouse support: detect and degrade.
- Read colors through one theme module that maps roles to ANSI indexes; never write an escape sequence or a hex value in a view.
- Test every screen with a light and a dark terminal theme and with `NO_COLOR=1`.
- Animations run on a ticker owned by the UI loop and stop when nothing is pending; never sleep in the render path.
- Every glyph has an ASCII fallback (`+-|`, `#`, `*`) selected when the locale is not UTF-8.
- Every animation is behind the motion preference check; test with it on.
- Read every color and style from `styles.go`; a view only calls styles and `lipgloss.Join*` / `Place`.
- Never pad with hand-counted spaces: widths come from `lipgloss.Width` and `.Width(n)`.

## Toolkit

| Design | Lip Gloss / Bubble Tea |
| Color token | one `lipgloss.AdaptiveColor{Light, Dark}` (or an ANSI index string `"4"` when the theme inherits the terminal) per token, in `styles.go` |
| Text style | one named `lipgloss.Style` per role (`Title`, `Muted`, `Key`, `Error`…), built once, never inline in a view |
| Frame | `.Border(lipgloss.RoundedBorder())` (or `NormalBorder`, `ThickBorder`, `DoubleBorder`, `BlockBorder`, `HiddenBorder`) + `.BorderForeground(token)` |
| Spacing | `.Padding(v, h)` inside frames, `.Margin(v, h)` between blocks; one cell is the unit |
| Layout | `lipgloss.JoinVertical` / `JoinHorizontal` compose blocks; `lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, view)` centers the whole screen |
| Width | `lipgloss.Width(s)` / `lipgloss.Height(s)` measure rendered blocks (display width, ANSI ignored); never `len(s)` |
| Size | the last `tea.WindowSizeMsg` is kept in the model; every view is laid out from it, never from a constant |
| Grid / keypad | a row of fixed-width cells (`.Width(n).Align(lipgloss.Center)`) joined horizontally; the focused cell gets the `Focus` style |
| Tables / lists | `lipgloss/table` for static tables, `bubbles/table` and `bubbles/list` for interactive ones |
| Help bar | `bubbles/help` + `bubbles/key` bindings; `help.Width = width` so it truncates with `…` instead of overflowing |
| Motion | `tea.Tick` for frames; `github.com/charmbracelet/harmonica` springs for eased values (position, flash fade) |

- The root view ends with `lipgloss.Place(...)` over the full window, so the composition is centered and never touches the edges by accident.
- Compose top-down: header, body, help bar as three blocks joined vertically; each block renders to exactly the width it was given.
- A block that does not fit swaps to its compact variant (smaller banner, fewer columns); test the 80×24 minimum with `teatest` golden files.
- Golden tests force a color profile (`lipgloss.SetColorProfile(termenv.Ascii)` or the `teatest` option) so snapshots do not depend on the terminal.
