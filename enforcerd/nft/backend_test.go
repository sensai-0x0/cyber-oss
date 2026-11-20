package nft

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/metacogna/sentinel-hunter/enforcerd"
)

func TestBackendUsesArgumentArrayAndRevokesSameElement(t *testing.T) {
	executor := &recordingExecutor{}
	backend := Backend{Executor: executor, Table: "sentinel", Set: "blocked"}
	action := enforcerd.Action{ID: "one", Kind: enforcerd.KindNFTables, Target: "192.0.2.0/24", TTL: time.Minute}
	if err := backend.Apply(context.Background(), action); err != nil {
		t.Fatal(err)
	}
	if err := backend.Revoke(context.Background(), action); err != nil {
		t.Fatal(err)
	}
	want := []call{
		{name: "nft", args: []string{"add", "element", "inet", "sentinel", "blocked", "{ 192.0.2.0/24 }"}},
		{name: "nft", args: []string{"delete", "element", "inet", "sentinel", "blocked", "{ 192.0.2.0/24 }"}},
	}
	if !reflect.DeepEqual(executor.calls, want) {
		t.Fatalf("calls = %#v, want %#v", executor.calls, want)
	}
}

func TestBackendRejectsInvalidTarget(t *testing.T) {
	backend := Backend{Executor: &recordingExecutor{}, Table: "sentinel", Set: "blocked"}
	action := enforcerd.Action{Target: "192.0.2.1; reboot"}
	if err := backend.Apply(context.Background(), action); err == nil {
		t.Fatal("shell-like target was accepted")
	}
}

type call struct {
	name string
	args []string
}

type recordingExecutor struct{ calls []call }

func (r *recordingExecutor) Run(_ context.Context, name string, args ...string) error {
	r.calls = append(r.calls, call{name: name, args: append([]string(nil), args...)})
	return nil
}
