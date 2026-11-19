# jupy

A small ncurses browser for Kaggle notebooks: list your notebooks, open one to
see its cells rendered as cards, edit a cell in `$VISUAL` / `$EDITOR` (nvim by
default), and run cells on a Kaggle GPU through a live Jupyter session.

```
logged in as: you
┌ Notebooks (40) ───────────┐ ┌ 1 py ────────────────────────────────┐
│   notebook72cbd09a56 11-19│ │ import torch                         │
│ > J*bJev             11-04│ │ import torch.nn as nn                │
│   notebooke38a2762d0 10-09│ └──────────────────────────────────────┘
└───────────────────────────┘ ┌ 2 py ────────────────────────────────┐
                              │ from torch.optim import Adam         │
                              │ loss_fn = nn.CrossEntropyLoss()      │
                              └──────────────────────────────────────┘
 ready - R runs the selected cell       connected: kkb-...kaggle.net - Tesla T4 x2
 j/k move  J/K scroll  tab pane  enter edit  R run  a add  m md  d del  c connect  q quit
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

| key | action |
|---|---|
| `j` / `k`, arrows | move |
| `g` / `G` | first / last |
| `tab`, `h` / `l` | switch pane |
| `enter` | open notebook, or edit the selected cell |
| `e` | edit the selected cell in the editor |
| `R` / `ctrl-r` | run the selected cell on the attached kernel |
| `a` / `m` | insert a code / markdown cell after the selection |
| `d` | delete the selected cell |
| `c` | attach to a Kaggle Jupyter session (prompts for the URL) |
| `J` / `K`, pgup/pgdn | scroll the cell stream |
| `r` | reload the open notebook |
| `n` | load the next page of notebooks |
| `w` | write the notebook (with your edits) to `./<slug>.ipynb` |
| `P` | push the buffer to Kaggle as a new version (no run) |
| `q` | quit |

## Credentials

Checked in this order:

1. `KAGGLE_USERNAME` + `KAGGLE_KEY` (HTTP Basic)
2. `KAGGLE_API_TOKEN` (Bearer)
3. `$KAGGLE_CONFIG_DIR/kaggle.json`, default `~/.kaggle/kaggle.json` (Basic)
4. `$KAGGLE_CONFIG_DIR/credentials.json` (OAuth, written by `kaggle auth login`) —
   used as a Bearer token, refreshed through `users.AccountService/GenerateAccessToken`
   when expired, and the result is cached back into the file

## Running cells on a Kaggle GPU

Kaggle's public API can push a notebook and start a *batch* run, but it does not
mint the interactive Jupyter URL — that one value only comes out of the browser.
So the session is started on kaggle.com, where you also pick the accelerator,
and `jupy` attaches to it:

1. Open the notebook on kaggle.com and start an interactive session with the
   accelerator you want (T4 x2, P100, TPU — the picker is in the session dialog).
2. `Run -> Kaggle Jupyter Server -> Manually Connect`, copy the *Colab compatible*
   or *VS Code compatible* URL.
3. In `jupy`, press `c` and paste it — or export it as `JUPY_SESSION_URL` and
   `jupy` attaches on startup.

`jupy` then lists the session's kernel (starting `python3` if none is running)
and probes the hardware with `nvidia-smi`, showing e.g. `Tesla T4 x2` in the
status line. `R` executes the selected cell over the kernel's websocket channel
and streams stdout, stderr, results and tracebacks into that cell's card. The
kernel keeps its state between cells, exactly as in the notebook UI; markdown
cells are skipped. The session ends when Kaggle idles it out, and the URL stops
working at that point.

The URL embeds a session JWT. `jupy` keeps it in memory only — it is never
written to disk, logged, or shown after the prompt — so treat it as a secret and
do not paste it into anything you keep around.

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

## Saving back to Kaggle

`P` calls `SaveKernel` with `kernel_execution_type: QUICK_SAVE`: a new version
is created and nothing is queued. The push reuses the settings of the version
you opened — private flag, GPU/TPU, internet, docker image, machine shape, and
the dataset/competition/kernel/model sources — so it will not quietly drop your
data or flip the notebook public. Code cell outputs are stripped and cell
sources are joined into one string, which is what the server expects.

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
