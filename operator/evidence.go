package operator

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/metacogna/sentinel-hunter/canonjson"
	"github.com/metacogna/sentinel-hunter/enforcerd"
)

// ErrEvidenceMismatch means the ledger export does not support the evidence the action cites.
var ErrEvidenceMismatch = errors.New("ledger evidence does not match the cited record")

const (
	genesis       = "0000000000000000000000000000000000000000000000000000000000000000"
	maxRecordSize = 1 << 20
)

// Evidence is the ledger record an approval commits to (ADR 0005).
type Evidence struct {
	Seq    int64
	Digest string
	// Lines are the facts shown to the operator before signing. Not signed.
	Lines []string
}

type ledgerRecord struct {
	Seq      int64          `json:"seq"`
	PrevHash string         `json:"prev_hash"`
	Hash     string         `json:"record_hash"`
	Stage    string         `json:"stage"`
	Scorer   string         `json:"scorer_id"`
	Intel    map[string]any `json:"intel"`
	Evidence map[string]any `json:"evidence"`
	Policy   struct {
		Action   string `json:"action"`
		ActionID string `json:"action_id"`
		Target   string `json:"target"`
		Broad    bool   `json:"broad"`
	} `json:"policy"`
}

func mismatch(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrEvidenceMismatch, fmt.Sprintf(format, args...))
}

// VerifyEvidence recomputes the hash chain from record 0 through seq in a ledger export
// (one canonical record per line, from `python -m orchestrator.ledger export`) and checks
// that record seq is a mitigation proposal for exactly this action, with the cited digest.
// Any break fails closed. It reads no further than seq.
func VerifyEvidence(export io.Reader, seq int64, digest string, action enforcerd.Action) (Evidence, error) {
	if seq < 0 {
		return Evidence{}, mismatch("negative sequence")
	}
	scanner := bufio.NewScanner(export)
	scanner.Buffer(make([]byte, 0, 64*1024), maxRecordSize)
	previous := genesis
	for index := int64(0); scanner.Scan(); index++ {
		line := scanner.Bytes()
		canonical, err := canonjson.Canonicalize(line)
		if err != nil || string(canonical) != string(line) {
			return Evidence{}, mismatch("record %d is not canonical", index)
		}
		var record ledgerRecord
		if err := json.Unmarshal(line, &record); err != nil {
			return Evidence{}, mismatch("record %d is unreadable", index)
		}
		recomputed, err := recordHash(line)
		if err != nil || record.Seq != index || record.PrevHash != previous || record.Hash != recomputed {
			return Evidence{}, mismatch("chain breaks at record %d", index)
		}
		previous = record.Hash
		if index < seq {
			continue
		}
		return citedEvidence(record, seq, digest, action)
	}
	if err := scanner.Err(); err != nil {
		return Evidence{}, mismatch("read export: %v", err)
	}
	return Evidence{}, mismatch("export ends before record %d", seq)
}

func citedEvidence(record ledgerRecord, seq int64, digest string, action enforcerd.Action) (Evidence, error) {
	switch {
	case record.Hash != digest:
		return Evidence{}, mismatch("record %d has a different digest", seq)
	case record.Stage != "propose" || record.Policy.Action != "mitigate":
		return Evidence{}, mismatch("record %d is not a mitigation proposal", seq)
	case record.Policy.ActionID != action.ID || record.Policy.Target != action.Target:
		return Evidence{}, mismatch("record %d describes a different action or target", seq)
	}
	return Evidence{Seq: seq, Digest: digest, Lines: evidenceLines(record)}, nil
}

func evidenceLines(record ledgerRecord) []string {
	lines := []string{
		fmt.Sprintf("ledger record %d  digest %s", record.Seq, record.Hash),
		fmt.Sprintf("proposal: %s %s (broad=%t) scored by %s", record.Policy.Action, record.Policy.Target, record.Policy.Broad, record.Scorer),
	}
	for _, section := range []struct {
		name  string
		value map[string]any
	}{{"intel", record.Intel}, {"evidence", record.Evidence}} {
		if encoded, err := encodeSection(section.value); err == nil && encoded != "" {
			lines = append(lines, section.name+": "+encoded)
		}
	}
	return lines
}

func encodeSection(value map[string]any) (string, error) {
	if len(value) == 0 {
		return "", nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	canonical, err := canonjson.Canonicalize(raw)
	return string(canonical), err
}

// recordHash is sha256 of the canonical record without its record_hash, as the ledger computes it.
func recordHash(line []byte) (string, error) {
	var body map[string]any
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.UseNumber()
	if err := decoder.Decode(&body); err != nil {
		return "", err
	}
	delete(body, "record_hash")
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	canonical, err := canonjson.Canonicalize(raw)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

// ApproveWithEvidence signs a v2 action that commits to a verified ledger record.
func ApproveWithEvidence(action enforcerd.Action, privateKey ed25519.PrivateKey, name string, ttl time.Duration, now time.Time, breadth enforcerd.Breadth, evidence Evidence) (enforcerd.Action, error) {
	action.Version = "v2"
	action.EvidenceDigest = evidence.Digest
	action.LedgerSeq = evidence.Seq
	return Approve(action, privateKey, name, ttl, now, breadth)
}
