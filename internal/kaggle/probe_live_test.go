package kaggle

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"testing"
)

func TestProbeReadOnly(t *testing.T) {
	if os.Getenv("JUPY_LIVE") == "" {
		t.Skip()
	}
	creds, _ := LoadCreds()
	post := func(method, body string) (int, string) {
		req, _ := http.NewRequest("POST", DefaultBaseURL+"/v1/kernels.KernelsApiService/"+method, bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+creds.Token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		if len(raw) > 700 {
			raw = raw[:700]
		}
		return resp.StatusCode, string(raw)
	}

	for _, c := range []struct{ method, body string }{
		{"GetAcceleratorQuotaStatistics", `{}`},
		{"GetKernelSessionStatus", `{"userName":"himanshusardana2","kernelSlug":"j-bjev"}`},
		{"ListKernelSessionOutput", `{"userName":"himanshusardana2","kernelSlug":"j-bjev","pageSize":5}`},
	} {
		code, body := post(c.method, c.body)
		t.Logf("%s -> %d\n%s", c.method, code, body)
	}
}
