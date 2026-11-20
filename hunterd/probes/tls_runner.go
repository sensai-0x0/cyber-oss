package probes

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/projectdiscovery/tlsx/pkg/tlsx"
	"github.com/projectdiscovery/tlsx/pkg/tlsx/clients"
)

// TLSRunner wraps the projectdiscovery/tlsx library.
type TLSRunner struct {
	service *tlsx.Service
	options *clients.Options
}

func NewTLSRunner(timeout time.Duration) (*TLSRunner, error) {
	seconds := max(1, int(timeout.Round(time.Second)/time.Second))
	options := &clients.Options{
		Timeout: seconds, Retries: 1, Jarm: true, Ja3: true, SAN: true, CN: true,
		DisableUpdateCheck: true,
	}
	service, err := tlsx.New(options)
	if err != nil {
		return nil, fmt.Errorf("initialize tlsx: %w", err)
	}
	return &TLSRunner{service: service, options: options}, nil
}

// Probe performs an active TLS handshake. The upstream API has no context;
// cancellation is therefore checked before the bounded-timeout call.
func (r *TLSRunner) Probe(ctx context.Context, host, ip, port string) (Observation, error) {
	if err := ctx.Err(); err != nil {
		return Observation{}, err
	}
	response, err := r.service.Connect(host, ip, port)
	if err != nil {
		return Observation{}, fmt.Errorf("tlsx probe: %w", err)
	}
	return normalizeTLSResponse(response), nil
}

func (r *TLSRunner) Close() {
	if r.options.Fastdialer != nil {
		r.options.Fastdialer.Close()
	}
}

func normalizeTLSResponse(response *clients.Response) Observation {
	host := response.IP
	if host == "" {
		host = response.Host
	}
	attributes := map[string]any{
		"host": response.Host,
		"ip":   response.IP,
	}
	if response.JarmHash != "" {
		attributes["jarm"] = response.JarmHash
	}
	if response.Ja3Hash != "" {
		attributes["ja3"] = response.Ja3Hash
	}
	var observedAt time.Time
	if response.Timestamp != nil {
		observedAt = *response.Timestamp
	}
	if certificate := response.CertificateResponse; certificate != nil {
		if certificate.SubjectCN != "" {
			attributes["common_name"] = certificate.SubjectCN
		}
		if len(certificate.SubjectAN) > 0 {
			attributes["sans"] = append([]string(nil), certificate.SubjectAN...)
		}
		if !certificate.NotAfter.IsZero() {
			attributes["certificate_not_after"] = certificate.NotAfter.Format(time.RFC3339)
		}
	}
	return newObservation("tlsx", net.JoinHostPort(host, response.Port), observedAt, attributes)
}
