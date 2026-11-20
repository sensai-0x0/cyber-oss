// Package rpz adapts mitigation actions to dynamic DNS RPZ updates.
package rpz

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/metacogna/sentinel-hunter/enforcerd"
)

type Executor interface {
	Run(context.Context, string, string, ...string) error
}

type ExecExecutor struct{}

func (ExecExecutor) Run(ctx context.Context, input, name string, args ...string) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Stdin = strings.NewReader(input)
	return command.Run()
}

type Backend struct {
	Executor Executor
	Zone     string
	Server   string
}

var dnsName = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9.-]*[a-zA-Z0-9])?$`)

func (b Backend) Apply(ctx context.Context, action enforcerd.Action) error {
	return b.update(ctx, action, true)
}

func (b Backend) Revoke(ctx context.Context, action enforcerd.Action) error {
	return b.update(ctx, action, false)
}

func (b Backend) update(ctx context.Context, action enforcerd.Action, add bool) error {
	if b.Executor == nil || !dnsName.MatchString(b.Zone) || !dnsName.MatchString(b.Server) || !dnsName.MatchString(action.Target) {
		return errors.New("rpz backend received an invalid name or configuration")
	}
	ttl := int64(action.TTL / time.Second)
	if ttl <= 0 || action.TTL > enforcerd.MaxTTL {
		return enforcerd.ErrInvalidTTL
	}
	operation := "update delete"
	record := "A"
	if add {
		operation = "update add"
		record = fmt.Sprintf("%d A 127.0.0.1", ttl)
	}
	input := fmt.Sprintf("server %s\nzone %s\n%s %s.%s. %s\nsend\n", b.Server, b.Zone, operation, action.Target, b.Zone, record)
	return b.Executor.Run(ctx, input, "nsupdate", "-v")
}
