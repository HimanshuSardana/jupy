package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/himanshusardana/jupy/internal/kaggle"
	"github.com/himanshusardana/jupy/internal/notebook"
	"github.com/himanshusardana/jupy/internal/ui"
)

func main() {
	user := flag.String("user", "", "list this Kaggle user's notebooks instead of your own")
	pageSize := flag.Int("page-size", 50, "notebooks fetched per page")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: jupy [-user name] [-page-size n]\n       jupy notebook.ipynb [more.ipynb ...]\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	var (
		cfg ui.Config
		err error
	)
	if flag.NArg() > 0 {
		cfg = localConfig(flag.Args())
	} else {
		cfg, err = remoteConfig(*user, *pageSize)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "jupy: %v\n", err)
		os.Exit(2)
	}

	if err := ui.Run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "jupy: %v\n", err)
		os.Exit(1)
	}
}

func localConfig(paths []string) ui.Config {
	kernels := make([]kaggle.Kernel, 0, len(paths))
	for _, p := range paths {
		name := filepath.Base(p)
		kernels = append(kernels, kaggle.Kernel{
			Ref:   p,
			Slug:  name,
			Title: strings.TrimSuffix(name, filepath.Ext(name)),
		})
	}
	return ui.Config{
		Notebooks: kernels,
		Open: func(k kaggle.Kernel) (*notebook.Notebook, error) {
			return notebook.Load(k.Ref)
		},
	}
}

func remoteConfig(user string, pageSize int) (ui.Config, error) {
	creds, err := kaggle.LoadCreds()
	if err != nil {
		return ui.Config{}, fmt.Errorf("%w\n\n  to browse local files instead: jupy notebook.ipynb", err)
	}
	client := kaggle.New(creds)

	nbs, next, err := client.ListKernels(user, "", pageSize)
	if err != nil {
		return ui.Config{}, err
	}

	cfg := ui.Config{
		User:      creds.Username,
		Notebooks: nbs,
		Open: func(k kaggle.Kernel) (*notebook.Notebook, error) {
			owner := k.Owner
			if owner == "" {
				owner = user
			}
			if owner == "" {
				owner = creds.Username
			}
			meta, source, err := client.GetKernel(owner, k.Slug)
			if err != nil {
				return nil, err
			}
			return notebook.Parse(meta.Title, meta.Ref, source)
		},
	}
	if next != "" {
		cfg.More = func() ([]kaggle.Kernel, error) {
			more, token, err := client.ListKernels(user, next, pageSize)
			if err != nil {
				return nil, err
			}
			next = token
			return more, nil
		}
	}
	return cfg, nil
}
