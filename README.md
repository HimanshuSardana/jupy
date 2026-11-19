# jupy

A small ncurses browser for Kaggle notebooks: list your notebooks, open one to
see its cells rendered as cards, edit a cell in `$VISUAL` / `$EDITOR` (nvim by
default), and run cells on a Kaggle GPU through a live Jupyter session.

```
  title                                        notebook             updated
──────────────────────────────────────────────────────────────────────────
. my notebooks (41)
> notebook175287c276                           notebook175287c276   2026-11-19
  J*bJev                                       j-bjev               2026-11-04
  notebooke38a2762d0                           notebooke38a2762d0   2026-10-09
 j/k move  enter open  a new  o browser  n more  ctrl-p commands  q quit
```

Pressing `enter` opens the notebook, and its cells fill the screen:

```
 [2] himanshusardana2/notebook175287c276 (2 cells)               esc back
┌ 1 py ──────────────────────────────────────────────────────────────────┐
│ print("hello world")                                                   │
└────────────────────────────────────────────────────────────────────────┘
┌ 2 py ──────────────────────────────────────────────────────────────────┐
│ print("second cell")                                                   │
└────────────────────────────────────────────────────────────────────────┘
 ready - R runs the selected cell       connected: kkb-...kaggle.net - Tesla T4 x2
 j/k move  J/K scroll  enter edit  R run  a/m add  d del  P push  esc back
```

## Build

cgo + ncurses development headers are required (`ncurses`/`ncursesw`, usually
already present on Linux/macOS).

```sh
go build -o jupy .
```

## Use

```sh
jupy                          # your notebooks, from api.kaggle.com
jupy -user someone            # another account's public notebooks
jupy notebook.ipynb [more...] # local files, no network
```

`ctrl-p` opens the command palette on either screen: type to filter, `j`/`k` or
arrows to move, `enter` runs, `esc` cancels, `ctrl-u` clears the query. The
selected row in a menu is drawn with a grey cursorline background (colour 236 on
256-colour terminals, reverse video elsewhere).

### Notebook table

| key | action |
|---|---|
| `j` / `k`, arrows, `g` / `G` | move the selection |
| `enter` / `l` | open the selected notebook |
| `a` | create a notebook (prompts for a title) |
| `o` | open the selected notebook on kaggle.com |
| `n` | load the next page of notebooks |

### Notebook screen

| key | action |
|---|---|
| `esc` / `h` | back to the table |
| `j` / `k`, arrows, `g` / `G` | move between cells |
| `J` / `K`, pgup/pgdn | scroll the cell stream |
| `enter` / `e` | edit the selected cell in the editor |
| `R` / `ctrl-r` | run the selected cell on the attached kernel |
| `a` / `m` | insert a code / markdown cell after the selection |
| `d` | delete the selected cell |
| `c` | start a session for the open notebook, pick the accelerator, attach |
| `C` | attach to a session by pasting its URL (fallback) |
| `x` | stop the session `c` started |
| `o` | open the notebook on kaggle.com in the browser |
| `r` | reload the open notebook |
| `w` | write the notebook (with your edits) to `./<slug>.ipynb` |
| `P` | push the buffer to Kaggle as a new version (no run) |

`q` quits from either screen.

## Credentials

Checked in this order:

1. `KAGGLE_USERNAME` + `KAGGLE_KEY` (HTTP Basic)
2. `KAGGLE_API_TOKEN` (Bearer)
3. `$KAGGLE_CONFIG_DIR/kaggle.json`, default `~/.kaggle/kaggle.json` (Basic)
4. `$KAGGLE_CONFIG_DIR/credentials.json` (OAuth, written by `kaggle auth login`) —
   used as a Bearer token, refreshed through `users.AccountService/GenerateAccessToken`
   when expired, and the result is cached back into the file

## Running cells on a Kaggle GPU

`c` starts the session itself. It asks for an accelerator (cpu, T4 x2, P100, L4,
TPU 1VM v3-8), calls `CreateKernelSession`, polls the returned long-running
operation, reads the Jupyter URL out of its response and attaches — no browser
step, no URL to paste.

The flow, for the record:

1. `POST /v1/kernels.KernelsApiService/CreateKernelSession` with
   `{slug: "owner/slug", language, kernelType, machineShape}` returns an
   `Operation` whose `metadata.kernelSessionId` names the session.
2. `POST /v1/common.OperationsService/GetOperation` with that operation's name
   until `done`; `response.jupyterUrl` and `response.tokenizedJupyterUrl` carry
   the proxy URL. The tokenised form is sometimes a bare host plus `?token=`, so
   the plain URL with that token is tried first, then the tokenised one.
3. Attach: list kernels (starting `python3` if none runs), probe the hardware
   with `nvidia-smi`, and stream cell output over the kernel's websocket.

`R` executes the selected cell; stdout, stderr, results and tracebacks land in
that cell's card, and the kernel keeps state between cells. `x` cancels the
session so it stops using quota.

Two limits come from Kaggle, not from jupy: only **one** interactive GPU session
is allowed per account (`Maximum interactive GPU session count of 1 reached`),
and a session for a notebook that already has one refuses with
`a session is already running` — in that case press `C` and paste the URL from
*Run -> Kaggle Jupyter Server*, or stop the session on kaggle.com first.

Either URL embeds a session credential. `jupy` keeps it in memory only — never
written to disk, logged, or redrawn after the prompt.

## Screens

The table is one row per notebook with its slug and last run, plus a group row
and a sort note. `a` creates a notebook from here: it prompts for a title, makes
it private on Kaggle with `QUICK_SAVE` (so nothing runs), and opens it so you can
add cells. The notebook screen is the cell stream, one box per cell, full width.

## Opening the notebook

`o` launches `https://www.kaggle.com/code/<owner>/<slug>` for the notebook you
have open, using `$BROWSER` if set, else `open` on macOS and `xdg-open`
elsewhere. Local `.ipynb` files have no Kaggle page, so the status line says so
instead.

## What runs where

- **Execution is remote.** `R` sends the cell source over the session's Jupyter
  websocket; the kernel on Kaggle' s machine runs it and streams output back.
  jupy never spawns a local interpreter.
- **The document is local.** Cell text is pulled once through the Kaggle API,
  then edited in memory. `a`/`m`/`d` change only that copy, `w` writes a local
  `.ipynb`, and `P` is the only thing that writes back to Kaggle.
- **The kernel is shared, the document is not.** Variables you create with `R`
  exist in the same kernel the browser is attached to, but the browser's
  notebook file comes from Kaggle's version store, so it keeps showing the old
  code until you push (or edit in the browser).
- **Output travels with the document.** A run's output exists in the kernel and
  in `jupy`'s cards; `w` and `P` both write it into the document first, so the
  local `.ipynb` and the pushed version carry it. `kaggle kernels push` sends
  cells with `outputs: []`, but the server accepts and keeps outputs, so a
  `P` version shows what the cells printed.

## Saving back to Kaggle

`P` calls `SaveKernel` with `kernel_execution_type: QUICK_SAVE`: a new version
is created and nothing is queued. The push reuses the settings of the version
you opened — private flag, GPU/TPU, internet, docker image, machine shape, and
the dataset/competition/kernel/model sources — so it will not quietly drop your
data or flip the notebook public. Cell sources are joined into one string for
the wire format, and cell outputs from your runs are included: the server keeps
them, so the version you push shows what the cells printed.

Reload the notebook page to see the new version. If you want the run too, push
with `SAVE_AND_RUN_ALL` (not wired to a key yet) or press "Save & Run All" in
the browser.

## How editing works

A cell's source is written to a temp file (`.py`, `.md`, `.R`, ... by cell type
and notebook language). When the cell has outputs, the editor opens them in a
vertical split — source left, output right (`nvim -O cell.py output.txt`); cells
without outputs open on their own. The terminal is handed to the editor and the
source is read back on exit, marked `edited` in the card label; `w` saves an
`.ipynb` copy locally. Nothing is pushed back to Kaggle — that needs a
`kernels push` round trip that this tool does not implement.

The TUI is drawn with narrow ncurses, which counts bytes rather than cells, so
sanitised text folds non-ASCII runes to `?`; box borders use the terminal's ACS
line-drawing characters instead of UTF-8 box glyphs for the same reason.

## API notes

Transport is the proto-JSON RPC layer (`POST https://api.kaggle.com/v1/kernels.KernelsApiService/ListKernels`),
because it is the layer whose schema is documented by the shipped `kagglesdk`
client and whose JSON field names are stable, clean camelCase. See
[`docs/kaggle-api-survey.md`](docs/kaggle-api-survey.md) for the full survey.

Two quirks that shape the UI:

- `ListKernels` rows carry only `ref`, `title`, `author` (a display name) and
  `lastRunTime`. Owner and slug are derived from `ref`; language, GPU, privacy
  and votes only exist after `GetKernel`, so the list cannot show them.
- `GetKernel` returns `blob.source`, the notebook document as a JSON string.

## Tests

```sh
go test ./...
JUPY_LIVE=1 go test ./internal/kaggle -run Live -v   # read-only, uses real creds
```

`JUPY_LIVE` exercises `ListKernels` + `GetKernel` + ipynb parsing against
api.kaggle.com.
