// operator approves or rejects JSON action requests without accessing a firewall.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/metacogna/sentinel-hunter/enforcerd"
	operatorcore "github.com/metacogna/sentinel-hunter/operator"
)

type wireAction struct {
	Version    string    `json:"schema_version"`
	ID         string    `json:"action_id"`
	Kind       string    `json:"kind"`
	Target     string    `json:"target"`
	Broad      bool      `json:"broad"`
	Approved   bool      `json:"approved"`
	ApprovedBy string    `json:"approved_by,omitempty"`
	TTLSeconds int64     `json:"ttl_seconds"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Signature  string    `json:"signature"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: operator keygen -out-prefix KEY | operator review -key PRIVATE -decision approve|reject -operator NAME -ttl 15m [-policy PATH] < action.json")
	}
	switch args[0] {
	case "keygen":
		flags := flag.NewFlagSet("keygen", flag.ContinueOnError)
		prefix := flags.String("out-prefix", "", "private/public key output prefix")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *prefix == "" {
			return errors.New("out-prefix is required")
		}
		public, private, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return err
		}
		if err := os.WriteFile(*prefix, private, 0o600); err != nil {
			return err
		}
		return os.WriteFile(*prefix+".pub", public, 0o644)
	case "review":
		flags := flag.NewFlagSet("review", flag.ContinueOnError)
		keyPath := flags.String("key", "", "Ed25519 private key")
		decision := flags.String("decision", "reject", "approve or reject")
		name := flags.String("operator", "", "operator identity")
		ttl := flags.Duration("ttl", 15*time.Minute, "bounded mitigation TTL")
		policyPath := flags.String("policy", "orchestrator/policy.yaml", "policy file holding the broad prefix thresholds")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		breadth, err := loadBreadth(*policyPath)
		if err != nil {
			return err
		}
		private, err := os.ReadFile(*keyPath)
		if err != nil {
			return err
		}
		var input wireAction
		if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
			return err
		}
		action := enforcerd.Action{ID: input.ID, Kind: enforcerd.Kind(input.Kind), Target: input.Target, Broad: input.Broad}
		var output enforcerd.Action
		if *decision == "approve" {
			output, err = operatorcore.Approve(action, private, *name, *ttl, time.Now(), breadth)
		} else if *decision == "reject" {
			output, err = operatorcore.Reject(action, private, *name, time.Now())
		} else {
			return errors.New("decision must be approve or reject")
		}
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(toWire(output))
	default:
		return errors.New("unknown operator command")
	}
}

func loadBreadth(path string) (enforcerd.Breadth, error) {
	file, err := os.Open(path)
	if err != nil {
		return enforcerd.Breadth{}, fmt.Errorf("open policy: %w", err)
	}
	defer func() { _ = file.Close() }()
	return enforcerd.ParseBreadth(file)
}

func toWire(action enforcerd.Action) wireAction {
	return wireAction{
		Version: "v1", ID: action.ID, Kind: string(action.Kind), Target: action.Target,
		Broad: action.Broad, Approved: action.Approved, ApprovedBy: action.ApprovedBy,
		TTLSeconds: int64(action.TTL / time.Second), CreatedAt: action.CreatedAt,
		ExpiresAt: action.ExpiresAt, Signature: action.Signature,
	}
}
