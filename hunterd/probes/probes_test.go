package probes

import (
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
