package kaggle

import "github.com/himanshusardana/jupy/internal/notebook"

func normalizeForLive(source []byte) ([]byte, error) {
	return notebook.NormalizeForPush(source)
}
