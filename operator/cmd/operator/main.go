// operator approves or rejects JSON action requests without accessing a firewall.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/metacogna/sentinel-hunter/enforcerd"
	operatorcore "github.com/metacogna/sentinel-hunter/operator"

	"github.com/metacogna/sentinel-hunter/hunterd/scope"
)

type wireAction struct {
	Version string `json:"schema_version"`
	ID      string `json:"action_id"`
	Kind    string `json:"kind"`
	Target  string `json:"target"`
	Broad   bool   `json:"broad"`
	// EvidenceDigest and LedgerSeq cite the ledger record shown to the operator (v2, ADR 0005).
	EvidenceDigest string    `json:"evidence_digest,omitempty"`
	LedgerSeq      *int64    `json:"ledger_seq,omitempty"`
	Approved       bool      `json:"approved"`
	ApprovedBy     string    `json:"approved_by,omitempty"`
	TTLSeconds     int64     `json:"ttl_seconds"`
	CreatedAt      time.Time `json:"created_at"`
	ExpiresAt      time.Time `json:"expires_at"`
	Signature      string    `json:"signature"`
}

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: operator keygen -out-prefix KEY | operator review ... | operator scope sign|verify ...")
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
		// SHR-010: a silent overwrite here destroys the trust root. Refuse instead.
		if err := writeNewFile(*prefix, private, 0o600); err != nil {
			return err
		}
		return writeNewFile(*prefix+".pub", public, 0o644)
	case "scope":
		if len(args) < 2 {
			return errors.New("usage: operator scope sign -key PRIVATE -key-id ID < bundle.json | operator scope verify -key PUBLIC -key-id ID < signed.json")
		}
		switch args[1] {
		case "sign":
			return runScopeSign(args[2:], stdin, stdout)
		case "verify":
			return runScopeVerify(args[2:], stdin, stdout)
		default:
			return fmt.Errorf("unknown scope command %q", args[1])
		}
	case "review":
		flags := flag.NewFlagSet("review", flag.ContinueOnError)
		keyPath := flags.String("key", "", "Ed25519 private key")
		decision := flags.String("decision", "reject", "approve or reject")
		name := flags.String("operator", "", "operator identity")
		ttl := flags.Duration("ttl", 15*time.Minute, "bounded mitigation TTL")
		ledgerPath := flags.String("ledger", "", "ledger export (python -m orchestrator.ledger export); signs v2 bound to the cited record")
		policyPath := flags.String("policy", "orchestrator/policy.yaml", "policy file holding the broad prefix thresholds")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		private, err := os.ReadFile(*keyPath)
		if err != nil {
			return err
		}
		var input wireAction
		if err := json.NewDecoder(stdin).Decode(&input); err != nil {
			return err
		}
		action := enforcerd.Action{ID: input.ID, Kind: enforcerd.Kind(input.Kind), Target: input.Target, Broad: input.Broad}
		var output enforcerd.Action
		if *decision == "approve" {
			// Only approval signs a breadth claim, so only approval needs the thresholds.
			breadth, loadErr := loadBreadth(*policyPath)
			if loadErr != nil {
				return loadErr
			}
			output, err = approve(action, input, private, *name, *ttl, breadth, *ledgerPath, stderr)
		} else if *decision == "reject" {
			output, err = operatorcore.Reject(action, private, *name, time.Now())
		} else {
			return errors.New("decision must be approve or reject")
		}
		if err != nil {
			return err
		}
		return json.NewEncoder(stdout).Encode(toWire(output))
	default:
		return errors.New("unknown operator command")
	}
}

// approve signs v2 when a ledger export is given and the action cites a record in it, and v1 otherwise.
// A citation without -ledger, or -ledger without a citation, is refused rather than dropped (ADR 0005).
func approve(action enforcerd.Action, input wireAction, private []byte, name string, ttl time.Duration, breadth enforcerd.Breadth, ledgerPath string, stderr io.Writer) (enforcerd.Action, error) {
	cited := input.LedgerSeq != nil || input.EvidenceDigest != ""
	switch {
	case ledgerPath == "" && cited:
		return enforcerd.Action{}, errors.New("action cites a ledger record; pass -ledger so it can be verified")
	case ledgerPath == "":
		fmt.Fprintln(stderr, "warning: signing v1, which carries no evidence binding")
		return operatorcore.Approve(action, private, name, ttl, time.Now(), breadth)
	case input.LedgerSeq == nil || input.EvidenceDigest == "":
		return enforcerd.Action{}, errors.New("-ledger needs ledger_seq and evidence_digest in the action")
	}
	file, err := os.Open(ledgerPath)
	if err != nil {
		return enforcerd.Action{}, fmt.Errorf("open ledger export: %w", err)
	}
	defer func() { _ = file.Close() }()
	evidence, err := operatorcore.VerifyEvidence(file, *input.LedgerSeq, input.EvidenceDigest, action)
	if err != nil {
		return enforcerd.Action{}, err
	}
	for _, line := range evidence.Lines {
		fmt.Fprintln(stderr, line)
	}
	return operatorcore.ApproveWithEvidence(action, private, name, ttl, time.Now(), breadth, evidence)
}

func loadBreadth(path string) (enforcerd.Breadth, error) {
	file, err := os.Open(path)
	if err != nil {
		return enforcerd.Breadth{}, fmt.Errorf("open policy: %w", err)
	}
	defer func() { _ = file.Close() }()
	return enforcerd.ParseBreadth(file)
}

// writeNewFile refuses to overwrite an existing file (SHR-010): a silent truncate of a key
// file destroys the trust root with no way back.
func writeNewFile(path string, data []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%s already exists; refusing to overwrite a key", path)
		}
		return err
	}
	defer func() { _ = file.Close() }()
	_, err = file.Write(data)
	return err
}

// wireScopeBundle is the unsigned CLI input/output shape for `scope sign`, matching
// schemas/scope_bundle.v1.json minus key_id and signature, which the command supplies and
// computes itself.
type wireScopeBundle struct {
	BundleID     string   `json:"bundle_id"`
	EngagementID string   `json:"engagement_id"`
	ScopeEpoch   uint64   `json:"scope_epoch"`
	NotBefore    string   `json:"not_before"`
	NotAfter     string   `json:"not_after"`
	AuthRef      string   `json:"auth_ref"`
	Targets      []string `json:"targets"`
	Exclusions   []string `json:"exclusions,omitempty"`
	Ports        []int    `json:"ports"`
	ProbeKinds   []string `json:"probe_kinds"`
	DomainWatch  []string `json:"domain_watchlist,omitempty"`
	Rate         struct {
		GlobalCPS       int `json:"global_cps"`
		PerBlockCPS     int `json:"per_block_cps"`
		PerIPPerMin     int `json:"per_ip_per_min"`
		PerWorkerMaxCPS int `json:"per_worker_max_cps"`
	} `json:"rate"`
}

func (w wireScopeBundle) toBundle() (scope.Bundle, error) {
	notBefore, err := time.Parse(time.RFC3339, w.NotBefore)
	if err != nil {
		return scope.Bundle{}, fmt.Errorf("parse not_before: %w", err)
	}
	notAfter, err := time.Parse(time.RFC3339, w.NotAfter)
	if err != nil {
		return scope.Bundle{}, fmt.Errorf("parse not_after: %w", err)
	}
	return scope.Bundle{
		BundleID: w.BundleID, EngagementID: w.EngagementID, ScopeEpoch: w.ScopeEpoch,
		NotBefore: notBefore, NotAfter: notAfter, AuthRef: w.AuthRef,
		Targets: w.Targets, Exclusions: w.Exclusions, Ports: w.Ports, ProbeKinds: w.ProbeKinds,
		DomainWatchlist: w.DomainWatch,
		Rate: scope.RateLimits{
			GlobalCPS: w.Rate.GlobalCPS, PerBlockCPS: w.Rate.PerBlockCPS,
			PerIPPerMin: w.Rate.PerIPPerMin, PerWorkerMaxCPS: w.Rate.PerWorkerMaxCPS,
		},
	}, nil
}

// runScopeSign reads an unsigned bundle as JSON on stdin and writes the signed bundle to
// stdout. HC-2 (plans/fleet/README.md) decides who holds -key and how; this command only
// signs what it is given.
func runScopeSign(args []string, stdin io.Reader, stdout io.Writer) error {
	flags := flag.NewFlagSet("scope sign", flag.ContinueOnError)
	keyPath := flags.String("key", "", "Ed25519 private scope key")
	keyID := flags.String("key-id", "", "key id recorded in the signed bundle")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *keyID == "" {
		return errors.New("-key-id is required")
	}
	private, err := os.ReadFile(*keyPath)
	if err != nil {
		return err
	}
	var input wireScopeBundle
	if err := json.NewDecoder(stdin).Decode(&input); err != nil {
		return err
	}
	bundle, err := input.toBundle()
	if err != nil {
		return err
	}
	signed, err := operatorcore.SignScope(ed25519.PrivateKey(private), *keyID, bundle)
	if err != nil {
		return err
	}
	_, err = stdout.Write(signed)
	return err
}

// runScopeVerify reads a signed bundle as JSON on stdin and reports whether it verifies right
// now against -key under -key-id. hunterd performs the authoritative check at startup; this
// command exists so an operator can check a bundle before distributing it.
func runScopeVerify(args []string, stdin io.Reader, stdout io.Writer) error {
	flags := flag.NewFlagSet("scope verify", flag.ContinueOnError)
	keyPath := flags.String("key", "", "Ed25519 public scope key")
	keyID := flags.String("key-id", "", "key id the bundle must be signed with")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *keyID == "" {
		return errors.New("-key-id is required")
	}
	public, err := os.ReadFile(*keyPath)
	if err != nil {
		return err
	}
	raw, err := io.ReadAll(stdin)
	if err != nil {
		return err
	}
	keys := map[string]ed25519.PublicKey{*keyID: ed25519.PublicKey(public)}
	bundle, err := scope.Parse(raw, keys, time.Now(), 0)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "ok: bundle %s for engagement %s, epoch %d, valid %s to %s\n",
		bundle.BundleID, bundle.EngagementID, bundle.ScopeEpoch,
		bundle.NotBefore.Format(time.RFC3339), bundle.NotAfter.Format(time.RFC3339))
	return err
}

func toWire(action enforcerd.Action) wireAction {
	version, seq := "v1", (*int64)(nil)
	if action.Version == "v2" {
		version, seq = "v2", &action.LedgerSeq
	}
	return wireAction{
		Version: version, EvidenceDigest: action.EvidenceDigest, LedgerSeq: seq, ID: action.ID, Kind: string(action.Kind), Target: action.Target,
		Broad: action.Broad, Approved: action.Approved, ApprovedBy: action.ApprovedBy,
		TTLSeconds: int64(action.TTL / time.Second), CreatedAt: action.CreatedAt,
		ExpiresAt: action.ExpiresAt, Signature: action.Signature,
	}
}
