# jupy

A terminal browser for Kaggle notebooks: a table of your notebooks, a cell view
for the one you open, and a live Kaggle kernel for running cells.

## Installation

Needs Go 1.22 or newer and the ncurses headers, since the UI is cgo.

```sh
# Debian, Ubuntu
sudo apt install libncurses-dev

# Arch
sudo pacman -S ncurses

# macOS
brew install ncurses

go build -o jupy .
./jupy
```

Credentials come from `KAGGLE_USERNAME`/`KAGGLE_KEY`, `KAGGLE_API_TOKEN`,
`~/.kaggle/kaggle.json` or `~/.kaggle/credentials.json`; without them jupy browses
local files only: `jupy notebook.ipynb`.

To run cells, press `c` and pick an accelerator, or `C` to paste the URL from
`Run > Kaggle Jupyter Server` on kaggle.com.

## Keybinds

Notebook table:

| key | action |
| --- | --- |
| `j` / `k`, arrows | move |
| `g` / `G` | first / last |
| `enter` / `l` | open the notebook |
| `a` | create a notebook |
| `o` | open it on kaggle.com |
| `n` | load more notebooks |
| `ctrl-p` | command palette |
| `q` | quit |

Notebook view:

| key | action |
| --- | --- |
| `esc` / `h` | back to the table |
| `j` / `k`, arrows | move between cells |
| `g` / `G` | first / last cell |
| `J` / `K`, pgup / pgdn | scroll |
| `enter` / `e` | edit the cell in `$EDITOR` |
| `R` / `ctrl-r` | run the cell on the kernel |
| `a` / `m` | add a code / markdown cell |
| `d` | delete the cell |
| `c` | start a session, pick an accelerator |
| `C` | attach by pasting a session URL |
| `x` | stop the session |
| `P` | push to Kaggle as a new version |
| `w` | write a local `.ipynb` copy |
| `r` | reload the notebook |
| `o` | open it on kaggle.com |
| `ctrl-p` | command palette |
| `q` | quit |

In a menu: `j` / `k` or `ctrl-n` / `ctrl-p` move, `enter` runs, `esc` closes,
typing filters the command palette.
