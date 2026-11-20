// Package nft adapts mitigation actions to an existing inet set.
package nft

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os/exec"
	"regexp"

	"github.com/metacogna/sentinel-hunter/enforcerd"
)

type Executor interface {
	Run(context.Context, string, ...string) error
}

type ExecExecutor struct{}

func (ExecExecutor) Run(ctx context.Context, name string, args ...string) error {
	return exec.CommandContext(ctx, name, args...).Run()
}

type Backend struct {
	Executor Executor
	Table    string
	Set      string
}

var identifier = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

func (b Backend) Apply(ctx context.Context, action enforcerd.Action) error {
	return b.run(ctx, "add", action)
}

func (b Backend) Revoke(ctx context.Context, action enforcerd.Action) error {
	return b.run(ctx, "delete", action)
}

func (b Backend) run(ctx context.Context, operation string, action enforcerd.Action) error {
	if b.Executor == nil || !identifier.MatchString(b.Table) || !identifier.MatchString(b.Set) {
		return errors.New("nft backend is not configured with safe identifiers")
	}
	if _, err := netip.ParsePrefix(action.Target); err != nil {
		if _, addrErr := netip.ParseAddr(action.Target); addrErr != nil {
			return fmt.Errorf("invalid nft target: %w", err)
		}
	}
	element := "{ " + action.Target + " }"
	return b.Executor.Run(ctx, "nft", operation, "element", "inet", b.Table, b.Set, element)
}
