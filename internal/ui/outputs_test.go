package ui

import (
	"encoding/json"
	"testing"

	"github.com/himanshusardana/jupy/internal/jupyter"
)

func TestIPynbOutputsGroupsStreams(t *testing.T) {
	events := []jupyter.Event{
		{Kind: jupyter.Stdout, Text: "epoch 1\n"},
		{Kind: jupyter.Stdout, Text: "loss 0.5\n"},
		{Kind: jupyter.Stderr, Text: "warn\n"},
		{Kind: jupyter.Result, Text: "42"},
		{Kind: jupyter.Error, Text: "Traceback\nValueError: bad"},
	}
	raw, err := json.Marshal(ipynbOutputs(events))
	if err != nil {
		t.Fatal(err)
	}
	var outs []map[string]any
	if err := json.Unmarshal(raw, &outs); err != nil {
		t.Fatal(err)
	}
	if len(outs) != 4 {
		t.Fatalf("outputs = %d, want 4 (two streams, a result and an error)", len(outs))
	}
	if outs[0]["output_type"] != "stream" || outs[0]["name"] != "stdout" {
		t.Errorf("first output = %v", outs[0])
	}
	if text, _ := outs[0]["text"].([]any); len(text) != 2 || text[0] != "epoch 1\n" {
		t.Errorf("stdout text = %v", outs[0]["text"])
	}
	if outs[1]["name"] != "stderr" {
		t.Errorf("second output = %v", outs[1])
	}
	if outs[2]["output_type"] != "execute_result" {
		t.Errorf("result output = %v", outs[2])
	}
	data, _ := outs[2]["data"].(map[string]any)
	if text, _ := data["text/plain"].([]any); len(text) != 1 || text[0] != "42" {
		t.Errorf("result data = %v", data)
	}
	if outs[3]["output_type"] != "error" {
		t.Errorf("error output = %v", outs[3])
	}
}

func TestIPynbOutputsEmpty(t *testing.T) {
	if got := ipynbOutputs(nil); len(got) != 0 {
		t.Errorf("outputs = %v, want none", got)
	}
}
