package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	cfg.Session = os.Getenv("JUPY_SESSION_URL")
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
		Open: func(k kaggle.Kernel) (*notebook.Notebook, kaggle.Kernel, error) {
			nb, err := notebook.Load(k.Ref)
			return nb, k, err
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
		Open: func(k kaggle.Kernel) (*notebook.Notebook, kaggle.Kernel, error) {
			owner := k.Owner
			if owner == "" {
				owner = user
			}
			if owner == "" {
				owner = creds.Username
			}
			meta, source, err := client.GetKernel(owner, k.Slug)
			if err != nil {
				return nil, kaggle.Kernel{}, err
			}
			nb, err := notebook.Parse(meta.Title, meta.Ref, source)
			return nb, meta, err
		},
		Push: func(meta kaggle.Kernel, text []byte) (int, error) {
			return client.SaveKernel(meta, text, "QUICK_SAVE")
		},
		StartSession: func(meta kaggle.Kernel, shape string) (kaggle.StartedSession, error) {
			internet := meta.Internet
			// the session API wants owner/slug, not the bare slug
			slug := meta.Ref
			if slug == "" {
				slug = meta.Owner + "/" + meta.Slug
			}
			return client.StartSession(context.Background(), kaggle.SessionRequest{
				Slug:         slug,
				Language:     meta.Language,
				KernelType:   meta.Type,
				MachineShape: shape,
				DockerImage:  meta.Docker,
				Internet:     &internet,
				Timeout:      3 * time.Minute,
			})
		},
		StopSession: func(id int) error {
			return client.CancelSession(id)
		},
		CreateNotebook: func(title string) (kaggle.Kernel, error) {
			return client.CreateNotebook(title)
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
