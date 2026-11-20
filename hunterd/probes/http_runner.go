package probes

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/projectdiscovery/httpx/runner"
)

// HTTPRunner configures httpx for favicon hashing and page classification.
type HTTPRunner struct{ timeout time.Duration }

func NewHTTPRunner(timeout time.Duration) *HTTPRunner { return &HTTPRunner{timeout: timeout} }

// Probe performs an active HTTP request against one authorized target.
func (r *HTTPRunner) Probe(ctx context.Context, target string) (Observation, error) {
	if err := ctx.Err(); err != nil {
		return Observation{}, err
	}
	seconds := max(1, int(r.timeout.Round(time.Second)/time.Second))
	var result *runner.Result
	options := &runner.Options{
		InputTargetHost: []string{target}, Threads: 1, Timeout: seconds, Retries: 1,
		ExtractTitle: true, StatusCode: true, OutputIP: true, OutputServerHeader: true,
		Favicon: true, KnowledgeBase: true, DisableUpdateCheck: true, DisableStdout: true,
		DisableStdin: true, Silent: true,
		OnResult: func(candidate runner.Result) {
			if candidate.Err == nil && result == nil {
				copy := candidate
				result = &copy
			}
		},
	}
	client, err := runner.New(options)
	if err != nil {
		return Observation{}, fmt.Errorf("initialize httpx: %w", err)
	}
	defer client.Close()
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			client.Interrupt()
		case <-done:
		}
	}()
	client.RunEnumeration()
	close(done)
	if err := ctx.Err(); err != nil {
		return Observation{}, err
	}
	if result == nil {
		return Observation{}, errors.New("httpx returned no successful result")
	}
	return normalizeHTTPResponse(*result), nil
}

func normalizeHTTPResponse(response runner.Result) Observation {
	attributes := map[string]any{
		"ip":          response.HostIP,
		"status_code": response.StatusCode,
	}
	if response.Title != "" {
		attributes["title"] = response.Title
	}
	if response.FavIconMMH3 != "" {
		attributes["favicon_mmh3"] = response.FavIconMMH3
	}
	if response.WebServer != "" {
		attributes["web_server"] = response.WebServer
	}
	if pageType, ok := response.KnowledgeBase["PageType"]; ok {
		attributes["page_type"] = pageType
	}
	return newObservation("httpx", response.URL, response.Timestamp, attributes)
}
