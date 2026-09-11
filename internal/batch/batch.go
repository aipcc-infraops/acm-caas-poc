// internal/batch/batch.go
package batch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
)

type Work struct {
	Name string
	Run  func(ctx context.Context) (string, error)
}

type Result struct {
	Name    string `json:"name"`
	Message string `json:"message"`
	OK      bool   `json:"ok"`
}

func Execute(ctx context.Context, work []Work, concurrency int, w io.Writer) []Result {
	if concurrency <= 0 {
		concurrency = 5
	}
	if concurrency > 20 {
		concurrency = 20
	}
	if len(work) > 0 && concurrency > len(work) {
		concurrency = len(work)
	}

	lines := make(chan string, len(work))
	results := make([]Result, len(work))
	var mu sync.Mutex
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	// single printer goroutine — no mutex needed on w
	printerDone := make(chan struct{})
	go func() {
		for line := range lines {
			fmt.Fprintln(w, line)
		}
		close(printerDone)
	}()

	for i, item := range work {
		i, item := i, item
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			msg, err := item.Run(ctx)
			r := Result{Name: item.Name, OK: err == nil}
			if err != nil {
				r.Message = err.Error()
				lines <- fmt.Sprintf("[%s] error: %s", item.Name, err.Error())
			} else {
				r.Message = msg
				lines <- fmt.Sprintf("[%s] %s", item.Name, msg)
			}
			mu.Lock()
			results[i] = r
			mu.Unlock()
		}()
	}

	wg.Wait()
	close(lines)
	<-printerDone

	return results
}

func PrintSummary(results []Result, w io.Writer) {
	nameW := len("NAME")
	for _, r := range results {
		if len(r.Name) > nameW {
			nameW = len(r.Name)
		}
	}
	msgW := len("MESSAGE")
	for _, r := range results {
		if len(r.Message) > msgW {
			msgW = len(r.Message)
		}
	}

	fmt.Fprintf(w, "\n%-*s  %-7s  %s\n", nameW, "NAME", "STATUS", "MESSAGE")
	fmt.Fprintf(w, "%s  %s  %s\n", strings.Repeat("-", nameW), "-------", strings.Repeat("-", msgW))

	ok, fail := 0, 0
	for _, r := range results {
		status := "ok"
		if !r.OK {
			status = "error"
			fail++
		} else {
			ok++
		}
		fmt.Fprintf(w, "%-*s  %-7s  %s\n", nameW, r.Name, status, r.Message)
	}
	fmt.Fprintf(w, "\n%d succeeded, %d failed\n", ok, fail)
}

func ToJSON(results []Result) ([]byte, error) {
	return json.MarshalIndent(results, "", "  ")
}
