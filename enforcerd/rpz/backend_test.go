package rpz

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/metacogna/sentinel-hunter/enforcerd"
)

func TestBackendCreatesAndDeletesTTLRecord(t *testing.T) {
	executor := &recordingExecutor{}
	backend := Backend{Executor: executor, Zone: "rpz.lab", Server: "127.0.0.1"}
	action := enforcerd.Action{Target: "phish.example", TTL: 5 * time.Minute}
	if err := backend.Apply(context.Background(), action); err != nil {
		t.Fatal(err)
	}
	if err := backend.Revoke(context.Background(), action); err != nil {
		t.Fatal(err)
	}
	if len(executor.inputs) != 2 || !strings.Contains(executor.inputs[0], "update add phish.example.rpz.lab. 300 A 127.0.0.1") {
		t.Fatalf("updates = %#v", executor.inputs)
	}
	if !strings.Contains(executor.inputs[1], "update delete phish.example.rpz.lab. A") {
		t.Fatalf("revoke = %q", executor.inputs[1])
	}
}

type recordingExecutor struct{ inputs []string }

func (r *recordingExecutor) Run(_ context.Context, input, _ string, _ ...string) error {
	r.inputs = append(r.inputs, input)
	return nil
}
