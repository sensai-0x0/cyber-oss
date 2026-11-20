package probes

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/projectdiscovery/fastdialer/fastdialer"
	"github.com/projectdiscovery/tlsx/pkg/tlsx"
	"github.com/projectdiscovery/tlsx/pkg/tlsx/clients"

	"github.com/metacogna/sentinel-hunter/hunterd/scope"
)

// Authorizer gates every probe before a dial (I1-I5). TLSRunner and HTTPRunner each define
// this one-method-plus-host-resolve seam themselves rather than importing an exported
// interface from hunterd/scope (go.md §2): they only ever need CheckHost.
type Authorizer interface {
	CheckHost(ctx context.Context, host string, port uint16, kind scope.Kind) error
}

// TLSRunner wraps the projectdiscovery/tlsx library.
type TLSRunner struct {
	service    *tlsx.Service
	options    *clients.Options
	authorizer Authorizer
}

// NewTLSRunner builds a runner that checks authorizer before every dial (L1) and, when
// authorizer also exposes a *networkpolicy.NetworkPolicy via NetworkPolicy(), wires it into
// tlsx's fastdialer as a second, independent check at the real socket address (L2). A nil
// authorizer is refused: there is no unauthorized-by-default path in this constructor.
func NewTLSRunner(timeout time.Duration, authorizer Authorizer) (*TLSRunner, error) {
	if authorizer == nil {
		return nil, fmt.Errorf("tls runner: an Authorizer is required (I1)")
	}
	seconds := max(1, int(timeout.Round(time.Second)/time.Second))
	options := &clients.Options{
		Timeout: seconds, Retries: 1, Jarm: true, Ja3: true, SAN: true, CN: true,
		DisableUpdateCheck: true,
	}
	if dialer, err := tlsFastdialer(authorizer); err != nil {
		return nil, err
	} else if dialer != nil {
		options.Fastdialer = dialer
	}
	service, err := tlsx.New(options)
	if err != nil {
		return nil, fmt.Errorf("initialize tlsx: %w", err)
	}
	return &TLSRunner{service: service, options: options, authorizer: authorizer}, nil
}

// tlsFastdialer builds the L2 dialer when authorizer is a *scope.Authorizer: it wires both the
// shared L2 NetworkPolicy (when the bundle has a CIDR entry) and a per-dial OnValidateTarget
// hook (always), so L2 holds even for a hostname-only bundle.
func tlsFastdialer(authorizer Authorizer) (*fastdialer.Dialer, error) {
	a, ok := authorizer.(*scope.Authorizer)
	if !ok {
		return nil, nil
	}
	dialer, err := fastdialer.NewDialer(fastdialer.Options{
		NetworkPolicy:    a.NetworkPolicy(),
		OnValidateTarget: a.OnValidateTarget(scope.KindTLS),
	})
	if err != nil {
		return nil, fmt.Errorf("build L2 fastdialer: %w", err)
	}
	return dialer, nil
}

// Probe performs an active TLS handshake, after checking the target against the Authorizer
// (L1, I1-I2). The upstream API has no context; cancellation is therefore checked before the
// bounded-timeout call.
func (r *TLSRunner) Probe(ctx context.Context, host, ip, port string) (Observation, error) {
	if err := ctx.Err(); err != nil {
		return Observation{}, err
	}
	dialHost, dialPort := ip, port
	if dialHost == "" {
		dialHost = host
	}
	authorizedPort, err := parseAuthorizedPort(dialPort)
	if err != nil {
		return Observation{}, err
	}
	if err := r.authorizer.CheckHost(ctx, dialHost, authorizedPort, scope.KindTLS); err != nil {
		return Observation{}, err
	}
	response, err := r.service.Connect(host, ip, port)
	if err != nil {
		return Observation{}, fmt.Errorf("tlsx probe: %w", err)
	}
	return normalizeTLSResponse(response), nil
}

func parseAuthorizedPort(raw string) (uint16, error) {
	port, err := strconv.ParseUint(raw, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("tls runner: invalid port %q: %w", raw, err)
	}
	return uint16(port), nil
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
