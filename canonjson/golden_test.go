package canonjson

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type golden struct {
	JSONCases []struct {
		Name      string `json:"name"`
		Input     string `json:"input"`
		Canonical string `json:"canonical"`
	} `json:"json_cases"`
	Ledger []string `json:"ledger"`
}

func load(t *testing.T) golden {
	t.Helper()
	raw, err := os.ReadFile("../schemas/mitigation_action.v2.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var g golden
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	return g
}

func TestCanonicalFormMatchesSharedVectors(t *testing.T) {
	for _, c := range load(t).JSONCases {
		got, err := Canonicalize([]byte(c.Input))
		if err != nil || string(got) != c.Canonical {
			t.Errorf("%s: got %q, %v; want %q", c.Name, got, err, c.Canonical)
		}
	}
}

// The ledger bodies were written by Python, so re-canonicalising must be the identity.
func TestLedgerBodiesAreFixedPoints(t *testing.T) {
	for i, body := range load(t).Ledger {
		got, err := Canonicalize([]byte(body))
		if err != nil || string(got) != body {
			t.Errorf("record %d: not a fixed point (%v)\n got %q\nwant %q", i, err, got, body)
		}
	}
}

func TestCanonicalizeRejectsWhatPythonCannotProduce(t *testing.T) {
	for _, bad := range []string{`{"a":1`, `NaN`, `{"a":1}{"b":2}`, "\"\xff\"", `{"a":1,"a":2}`, ``} {
		if _, err := Canonicalize([]byte(bad)); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestOrderedObjectKeepsInsertionOrder(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, Object{{"z", 1}, {"a", "é<"}}); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(buf.String()); got != `{"z":1,"a":"é<"}` {
		t.Fatalf("got %s", got)
	}
}
