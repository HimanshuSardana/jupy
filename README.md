# jupy

A small ncurses browser for Kaggle notebooks: list your notebooks, open one to
see its cells, and edit a cell in `$VISUAL` / `$EDITOR` (nvim by default).

```
jupy  @you  Titanic EDA                      Notebooks (40) | Cells (5)
 titanic-eda      2026-11-19 |    1 py  import torch
 j-bjev           2026-11-04 | *  2 py  from torch.optim import Adam
 notebooke38a2    2026-10-09 |    3 py  from pathlib import Path
                             | Cell 2 of 5 - code
                             | from torch.optim import Adam
                             | loss_fn = nn.CrossEntropyLoss()
 cell 2 updated in the buffer - w writes a .ipynb copy
 j/k move  tab pane  enter open  e edit in nvim  r reload  n more  w save  q quit
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
| `J` / `K` | scroll the cell preview |
| `r` | reload the open notebook |
| `n` | load the next page of notebooks |
| `w` | write the notebook (with your edits) to `./<slug>.ipynb` |
| `q` | quit |

## Credentials

Checked in this order:

1. `KAGGLE_USERNAME` + `KAGGLE_KEY` (HTTP Basic)
2. `KAGGLE_API_TOKEN` (Bearer)
3. `$KAGGLE_CONFIG_DIR/kaggle.json`, default `~/.kaggle/kaggle.json` (Basic)
4. `$KAGGLE_CONFIG_DIR/credentials.json` (OAuth, written by `kaggle auth login`) —
   used as a Bearer token, refreshed through `users.AccountService/GenerateAccessToken`
   when expired, and the result is cached back into the file

## How editing works

A cell's source is written to a temp file (`.py`, `.md`, `.R`, ... by cell type
and notebook language), the terminal is handed to the editor, and the file is
read back on exit. Edits live in memory and are marked `*` in the cell list;
`w` saves an `.ipynb` copy locally. Nothing is pushed back to Kaggle — that
needs a `kernels push` round trip that this tool does not implement.

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
