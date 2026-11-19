package ui

import (
	"testing"

	"github.com/himanshusardana/jupy/internal/kaggle"
)

func TestNotebookURL(t *testing.T) {
	cases := []struct {
		name string
		meta kaggle.Kernel
		want string
	}{
		{"from ref", kaggle.Kernel{Ref: "me/titanic-eda", Slug: "titanic-eda"}, "https://www.kaggle.com/code/me/titanic-eda"},
		{"from owner and slug", kaggle.Kernel{Owner: "me", Slug: "x"}, "https://www.kaggle.com/code/me/x"},
		{"slug only is not a page", kaggle.Kernel{Slug: "x"}, ""},
		{"local file has no page", kaggle.Kernel{Ref: "/tmp/run.ipynb"}, ""},
		{"empty", kaggle.Kernel{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := notebookURL(tc.meta); got != tc.want {
				t.Errorf("notebookURL(%+v) = %q, want %q", tc.meta, got, tc.want)
			}
		})
	}
}
