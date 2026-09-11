// internal/batch/batch_test.go
package batch_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/pablofelix/acm-caas-poc/internal/batch"
)

func TestExecuteAllSucceed(t *testing.T) {
	work := []batch.Work{
		{Name: "c1", Run: func(ctx context.Context) (string, error) { return "ok1", nil }},
		{Name: "c2", Run: func(ctx context.Context) (string, error) { return "ok2", nil }},
	}
	var buf bytes.Buffer
	results := batch.Execute(context.Background(), work, 5, &buf)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if !r.OK {
			t.Errorf("expected %s to be OK", r.Name)
		}
	}
	out := buf.String()
	if !strings.Contains(out, "[c1]") || !strings.Contains(out, "[c2]") {
		t.Errorf("expected streaming output to contain cluster names, got: %s", out)
	}
}

func TestExecutePartialFailure(t *testing.T) {
	work := []batch.Work{
		{Name: "c1", Run: func(ctx context.Context) (string, error) { return "done", nil }},
		{Name: "c2", Run: func(ctx context.Context) (string, error) { return "", errors.New("boom") }},
	}
	results := batch.Execute(context.Background(), work, 5, &bytes.Buffer{})
	ok := 0
	for _, r := range results {
		if r.OK {
			ok++
		}
	}
	if ok != 1 {
		t.Errorf("expected 1 success, got %d", ok)
	}
}

func TestExecuteConcurrencyClampedToMax(t *testing.T) {
	work := make([]batch.Work, 3)
	for i := range work {
		i := i
		work[i] = batch.Work{Name: "c", Run: func(ctx context.Context) (string, error) { return "ok", nil }}
	}
	batch.Execute(context.Background(), work, 100, &bytes.Buffer{})
	// passes if it doesn't deadlock
}

func TestPrintSummary(t *testing.T) {
	results := []batch.Result{
		{Name: "c1", Message: "imported", OK: true},
		{Name: "c2", Message: "already exists", OK: false},
	}
	var buf bytes.Buffer
	batch.PrintSummary(results, &buf)
	out := buf.String()
	if !strings.Contains(out, "1 succeeded") || !strings.Contains(out, "1 failed") {
		t.Errorf("unexpected summary: %s", out)
	}
	if !strings.Contains(out, "NAME") || !strings.Contains(out, "STATUS") {
		t.Errorf("missing table headers: %s", out)
	}
}

func TestToJSON(t *testing.T) {
	results := []batch.Result{{Name: "c1", OK: true, Message: "done"}}
	data, err := batch.ToJSON(results)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"name": "c1"`) {
		t.Errorf("unexpected JSON: %s", data)
	}
}
