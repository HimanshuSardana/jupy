package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/himanshusardana/jupy/internal/jupyter"
	"github.com/himanshusardana/jupy/internal/notebook"
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

// A run's output must survive save and push: applyOutputs -> Marshal ->
// NormalizeForPush, the chain the push button uses.
func TestOutputsSurvivePushChain(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "notebook", "testdata", "sample.ipynb"))
	if err != nil {
		t.Fatal(err)
	}
	nb, err := notebook.Parse("t", "t", src)
	if err != nil {
		t.Fatal(err)
	}
	events := map[int][]jupyter.Event{
		1: {
			{Kind: jupyter.Stdout, Text: "hello from the kernel\n"},
			{Kind: jupyter.Result, Text: "42"},
		},
	}

	app := &app{nb: nb, outputs: events}
	app.applyOutputs()

	doc, err := nb.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	pushed, err := notebook.NormalizeForPush(doc)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(pushed, &parsed); err != nil {
		t.Fatal(err)
	}
	cells, _ := parsed["cells"].([]any)
	cell, _ := cells[1].(map[string]any)
	outs, _ := cell["outputs"].([]any)
	if len(outs) != 2 {
		t.Fatalf("pushed outputs = %v, want the stream and the result", outs)
	}
	stream, _ := outs[0].(map[string]any)
	if stream["output_type"] != "stream" {
		t.Errorf("first output = %v", stream)
	}
	if text, _ := stream["text"].([]any); len(text) != 1 || text[0] != "hello from the kernel\n" {
		t.Errorf("stream text = %v", stream["text"])
	}
	result, _ := outs[1].(map[string]any)
	if result["output_type"] != "execute_result" {
		t.Errorf("second output = %v", result)
	}
}
