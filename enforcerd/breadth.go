package enforcerd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
)

// ErrBroadMismatch means a claimed broad flag disagrees with the breadth derived from the target.
var ErrBroadMismatch = errors.New("claimed broad flag disagrees with the target")

var hostname = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)*[a-z](?:[a-z0-9-]{0,61}[a-z0-9])?\.?$`)

// Breadth holds the prefix lengths at or below which a range is broad (I14).
// The values come from orchestrator/policy.yaml via ParseBreadth, never from a request.
type Breadth struct{ V4, V6 int }

// ParseBreadth reads the top-level broad_prefix_threshold and broad_prefix_threshold_v6 keys
// from policy.yaml, so Go and Python share one source. Indented (nested) keys are not the
// policy's and are skipped; a # comment counts only at line start or after whitespace, as in
// YAML. It is a line reader, not YAML: where it cannot be sure it agrees with yaml.safe_load
// (a repeated key, a quoted key) it fails closed, so an approval stops rather than diverges.
func ParseBreadth(r io.Reader) (Breadth, error) {
	var b Breadth
	seen := map[string]bool{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := stripComment(scanner.Text())
		key, value, ok := strings.Cut(line, ":")
		if !ok || key != strings.TrimSpace(key) {
			continue // not a key, or nested under another key
		}
		if key != "broad_prefix_threshold" && key != "broad_prefix_threshold_v6" {
			continue
		}
		if seen[key] {
			return Breadth{}, fmt.Errorf("repeated %s in policy", key)
		}
		n, err := strconv.Atoi(strings.TrimSpace(value))
		limit := 32
		if key == "broad_prefix_threshold_v6" {
			limit = 128
		}
		if err != nil || n < 1 || n >= limit {
			return Breadth{}, fmt.Errorf("invalid %s in policy", key)
		}
		seen[key] = true
		if key == "broad_prefix_threshold" {
			b.V4 = n
		} else {
			b.V6 = n
		}
	}
	if err := scanner.Err(); err != nil {
		return Breadth{}, fmt.Errorf("read policy: %w", err)
	}
	if !seen["broad_prefix_threshold"] || !seen["broad_prefix_threshold_v6"] {
		return Breadth{}, errors.New("policy lacks broad prefix thresholds")
	}
	return b, nil
}

// stripComment drops a YAML comment: a # at the start of the line or after a space or tab.
func stripComment(line string) string {
	for i, r := range line {
		if r == '#' && (i == 0 || line[i-1] == ' ' || line[i-1] == '\t') {
			return line[:i]
		}
	}
	return line
}

func (b Breadth) valid() bool { return b.V4 >= 1 && b.V4 < 32 && b.V6 >= 1 && b.V6 < 128 }

// Derive reports whether target is broad using the target alone. A range at or below
// the threshold is broad; single addresses and hostnames never are. Anything that is
// not a plain address, CIDR or hostname is rejected rather than guessed at.
func (b Breadth) Derive(target string) (bool, error) {
	if !b.valid() {
		return false, errors.New("breadth thresholds are not configured")
	}
	if prefix, err := netip.ParsePrefix(target); err == nil {
		addr, bits, limit := prefix.Addr(), prefix.Bits(), b.V4
		if addr.Zone() != "" {
			return false, fmt.Errorf("%w: zoned addresses are not accepted", ErrInvalidAction)
		}
		if addr.Is4In6() {
			// An IPv4-mapped range is judged by its IPv4 width, not the looser IPv6 one.
			if bits < 96 {
				return false, fmt.Errorf("%w: range straddles the IPv4-mapped block", ErrInvalidAction)
			}
			addr, bits = addr.Unmap(), bits-96
		} else if addr.Is6() {
			limit = b.V6
		}
		return bits < addr.BitLen() && bits <= limit, nil
	}
	if addr, err := netip.ParseAddr(target); err == nil {
		if addr.Zone() != "" {
			return false, fmt.Errorf("%w: zoned addresses are not accepted", ErrInvalidAction)
		}
		return false, nil
	}
	if len(target) <= 253 && hostname.MatchString(strings.ToLower(target)) {
		return false, nil
	}
	return false, fmt.Errorf("%w: target is not an address, range or hostname", ErrInvalidAction)
}

// Check returns ErrBroadMismatch unless claimed equals the breadth derived from target.
func (b Breadth) Check(target string, claimed bool) error {
	derived, err := b.Derive(target)
	if err != nil {
		return err
	}
	if derived != claimed {
		return fmt.Errorf("%w: derived broad=%t", ErrBroadMismatch, derived)
	}
	return nil
}
