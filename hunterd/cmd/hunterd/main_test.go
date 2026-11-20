package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunNDJSONStreamsValidEvents(t *testing.T) {
	input := strings.Join([]string{
		`{"schema_version":"v1","event_id":"one","kind":"novel","source":"tlsx","target":"192.0.2.1:443","observed_at":"2026-01-02T03:04:05Z","fingerprint":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","attributes":{}}`,
		`{"schema_version":"v1","event_id":"two","kind":"mutated","source":"httpx","target":"https://192.0.2.2/","observed_at":"2026-01-02T03:04:06Z","fingerprint":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","attributes":{}}`,
	}, "\n") + "\n"
	var output bytes.Buffer
	if err := runNDJSON(context.Background(), strings.NewReader(input), &output, 1); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], `"event_id":"one"`) || !strings.Contains(lines[1], `"event_id":"two"`) {
		t.Fatalf("output = %q", output.String())
	}
}

func TestRunNDJSONRejectsMalformedEvent(t *testing.T) {
	input := `{"schema_version":"v1","event_id":"broken"}` + "\n"
	if err := runNDJSON(context.Background(), strings.NewReader(input), &bytes.Buffer{}, 1); err == nil {
		t.Fatal("expected malformed event to fail closed")
	}
}

func TestRunNDJSONHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := runNDJSON(ctx, strings.NewReader(""), &bytes.Buffer{}, 1)
	if err == nil {
		t.Fatal("expected canceled context error")
	}
}
