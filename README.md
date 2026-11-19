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
