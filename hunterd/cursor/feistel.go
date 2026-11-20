// Package cursor provides constant-memory, keyed permutation of IPv4 CIDRs.
package cursor

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net/netip"
)

const rounds = 4

// Permute applies a four-round balanced Feistel network within a bits-wide
// domain. The construction is a bijection, not an encryption API.
func Permute(value uint32, bits uint, key [32]byte) uint32 {
	if bits == 0 || bits > 32 || (bits < 32 && value >= uint32(1)<<bits) {
		panic("cidr: permutation input outside domain")
	}
	leftBits := (bits + 1) / 2
	rightBits := bits / 2
	left := value >> rightBits
	right := value & mask(rightBits)
	for round := uint(0); round < rounds; round++ {
		left, right = right, left^roundFunction(key, round, bits, leftBits, right, rightBits)&mask(leftBits)
		leftBits, rightBits = rightBits, leftBits
	}
	return left<<rightBits | right
}

// Unpermute reverses Permute within the same domain and with the same key.
func Unpermute(value uint32, bits uint, key [32]byte) uint32 {
	if bits == 0 || bits > 32 || (bits < 32 && value >= uint32(1)<<bits) {
		panic("cidr: permutation input outside domain")
	}
	leftBits := (bits + 1) / 2
	rightBits := bits / 2
	left := value >> rightBits
	right := value & mask(rightBits)
	for round := uint(rounds - 1); ; round-- {
		left, right = right^roundFunction(key, round, bits, rightBits, left, leftBits)&mask(rightBits), left
		leftBits, rightBits = rightBits, leftBits
		if round == 0 {
			break
		}
	}
	return left<<rightBits | right
}

func roundFunction(key [32]byte, round, domainBits, outputBits uint, value uint32, inputBits uint) uint32 {
	mac := hmac.New(sha256.New, key[:])
	var domain [8]byte
	domain[0] = byte(round)
	domain[1] = byte(domainBits)
	domain[2] = byte(outputBits)
	domain[3] = byte(inputBits)
	binary.BigEndian.PutUint32(domain[4:8], value)
	_, _ = mac.Write(domain[:])
	return binary.BigEndian.Uint32(mac.Sum(nil)[:4])
}

func mask(bits uint) uint32 {
	if bits == 0 {
		return 0
	}
	if bits == 32 {
		return ^uint32(0)
	}
	return uint32(1)<<bits - 1
}

// Range iterates every IPv4 address in a CIDR exactly once. It retains only
// the prefix, key, and next sequence number, so memory use is O(1).
type Range struct {
	prefix   netip.Prefix
	key      [32]byte
	hostBits uint
	next     uint64
	count    uint64
}

func NewRange(cidr string, key [32]byte) (*Range, error) {
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return nil, fmt.Errorf("parse CIDR: %w", err)
	}
	if !prefix.Addr().Is4() {
		return nil, fmt.Errorf("only IPv4 CIDRs are supported")
	}
	hostBits := uint(32 - prefix.Bits())
	return &Range{
		prefix:   prefix.Masked(),
		key:      key,
		hostBits: hostBits,
		count:    uint64(1) << hostBits,
	}, nil
}

// Next returns the next permuted address and false after exhaustion.
func (r *Range) Next() (netip.Addr, bool) {
	if r.next >= r.count {
		return netip.Addr{}, false
	}
	offset := Permute(uint32(r.next), r.hostBits, r.key)
	r.next++
	base := binary.BigEndian.Uint32(r.prefix.Addr().AsSlice())
	value := base + offset
	var raw [4]byte
	binary.BigEndian.PutUint32(raw[:], value)
	return netip.AddrFrom4(raw), true
}

// Cursor is the serializable position suitable for a BadgerDB checkpoint.
func (r *Range) Cursor() uint64 { return r.next }

// Restore resumes a range at a previously persisted sequence position.
func (r *Range) Restore(cursor uint64) error {
	if cursor > r.count {
		return fmt.Errorf("cursor %d exceeds range size %d", cursor, r.count)
	}
	r.next = cursor
	return nil
}
