package probes

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"testing"
	"time"

	httpxrunner "github.com/projectdiscovery/httpx/runner"
	"github.com/projectdiscovery/tlsx/pkg/tlsx/clients"
)

func TestNormalizeTLSResponse(t *testing.T) {
	notAfter := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	response := &clients.Response{
		Host: "staging.example", IP: "192.0.2.10", Port: "443", JarmHash: "jarm-a", Ja3Hash: "ja3-a",
		CertificateResponse: &clients.CertificateResponse{
			SubjectCN: "staging.example", SubjectAN: []string{"staging.example", "www.staging.example"}, NotAfter: notAfter,
		},
	}
	got := normalizeTLSResponse(response)
	if got.Target != "192.0.2.10:443" || got.Source != "tlsx" {
		t.Fatalf("identity = %+v", got)
	}
	if got.Attributes["jarm"] != "jarm-a" || got.Attributes["ja3"] != "ja3-a" || got.Attributes["common_name"] != "staging.example" {
		t.Fatalf("TLS attributes = %+v", got.Attributes)
	}
	if got.Attributes["certificate_not_after"] != notAfter.Format(time.RFC3339) {
		t.Fatalf("certificate expiry = %v", got.Attributes["certificate_not_after"])
	}
	sans, ok := got.Attributes["sans"].([]string)
	if !ok || len(sans) != 2 {
		t.Fatalf("SANs = %#v", got.Attributes["sans"])
	}
}

func TestNormalizeHTTPResponse(t *testing.T) {
	response := httpxrunner.Result{
		URL: "https://staging.example/login", HostIP: "192.0.2.10", StatusCode: 200,
		Title: "Sign in", FavIconMMH3: "123456", WebServer: "nginx",
		KnowledgeBase: map[string]any{"PageType": "login"},
	}
	got := normalizeHTTPResponse(response)
	if got.Target != "https://staging.example/login" || got.Source != "httpx" {
		t.Fatalf("identity = %+v", got)
	}
	for key, want := range map[string]any{"status_code": 200, "title": "Sign in", "favicon_mmh3": "123456", "page_type": "login", "ip": "192.0.2.10"} {
		if got.Attributes[key] != want {
			t.Errorf("attribute %q = %#v, want %#v", key, got.Attributes[key], want)
		}
	}
}

// The Python provenance allowlist (orchestrator/provenance.py) is compared with the
// same file, so the two languages cannot drift apart without a test failing.
func TestAttributeKeysMatchContract(t *testing.T) {
	raw, err := os.ReadFile("testdata/attribute_keys.json")
	if err != nil {
		t.Fatal(err)
	}
	var contract map[string][]string
	if err := json.Unmarshal(raw, &contract); err != nil {
		t.Fatal(err)
	}
	notAfter := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	tlsResponse := &clients.Response{
		Host: "staging.example", IP: "192.0.2.10", Port: "443", JarmHash: "j", Ja3Hash: "h",
		CertificateResponse: &clients.CertificateResponse{
			SubjectCN: "staging.example", SubjectAN: []string{"staging.example"}, NotAfter: notAfter,
		},
	}
	httpResponse := httpxrunner.Result{
		URL: "https://staging.example/", HostIP: "192.0.2.10", StatusCode: 200, Title: "t",
		FavIconMMH3: "1", WebServer: "nginx", KnowledgeBase: map[string]any{"PageType": "login"},
	}
	emitted := map[string][]string{
		"tlsx":  keysOf(normalizeTLSResponse(tlsResponse).Attributes),
		"httpx": keysOf(normalizeHTTPResponse(httpResponse).Attributes),
	}
	for source, want := range contract {
		if !reflect.DeepEqual(emitted[source], want) {
			t.Errorf("%s attribute keys = %v, contract %v", source, emitted[source], want)
		}
	}
	if len(contract) != len(emitted) {
		t.Errorf("contract sources %v, emitted %v", contract, emitted)
	}
}

func keysOf(attributes map[string]any) []string {
	keys := make([]string, 0, len(attributes))
	for key := range attributes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
