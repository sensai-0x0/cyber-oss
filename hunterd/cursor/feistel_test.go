package cursor

import (
	"net/netip"
	"strconv"
	"testing"
)

func TestPermuteIsBijectiveAcrossWidths(t *testing.T) {
	key := testKey()
	for _, bits := range []uint{1, 2, 3, 15, 16, 17, 31, 32} {
		t.Run(strconv.FormatUint(uint64(bits), 10), func(t *testing.T) {
			limit := uint64(1) << bits
			if bits > 16 {
				limit = 1 << 16 // Sample wide domains; exhaust small domains.
			}
			seen := make(map[uint32]struct{}, limit)
			for i := uint64(0); i < limit; i++ {
				value := uint32(i)
				permuted := Permute(value, bits, key)
				if bits < 32 && uint64(permuted) >= uint64(1)<<bits {
					t.Fatalf("Permute(%d, %d) = %d outside domain", value, bits, permuted)
				}
				if _, duplicate := seen[permuted]; duplicate {
					t.Fatalf("duplicate permutation %d at input %d", permuted, value)
				}
				seen[permuted] = struct{}{}
				if got := Unpermute(permuted, bits, key); got != value {
					t.Fatalf("round trip (%d, %d): got %d", value, bits, got)
				}
			}
		})
	}
}

func TestPermuteIsDeterministicAndKeyed(t *testing.T) {
	var other [32]byte
	other[0] = 1
	if got, want := Permute(12345, 32, testKey()), Permute(12345, 32, testKey()); got != want {
		t.Fatalf("not deterministic: %d != %d", got, want)
	}
	if Permute(12345, 32, testKey()) == Permute(12345, 32, other) {
		t.Fatal("different keys produced the same test-vector output")
	}
}

func TestRangeRandomizesWithoutDuplicatesAndResumes(t *testing.T) {
	r, err := NewRange("192.0.2.0/24", testKey())
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[netip.Addr]struct{}, 256)
	for i := 0; i < 100; i++ {
		addr, ok := r.Next()
		if !ok {
			t.Fatalf("range ended at %d", i)
		}
		if _, duplicate := seen[addr]; duplicate {
			t.Fatalf("duplicate address %s", addr)
		}
		seen[addr] = struct{}{}
	}
	cursor := r.Cursor()

	resumed, err := NewRange("192.0.2.0/24", testKey())
	if err != nil {
		t.Fatal(err)
	}
	if err := resumed.Restore(cursor); err != nil {
		t.Fatal(err)
	}
	for i := 100; i < 256; i++ {
		addr, ok := resumed.Next()
		if !ok {
			t.Fatalf("resumed range ended at %d", i)
		}
		if _, duplicate := seen[addr]; duplicate {
			t.Fatalf("duplicate address after resume %s", addr)
		}
		seen[addr] = struct{}{}
	}
	if len(seen) != 256 {
		t.Fatalf("visited %d addresses, want 256", len(seen))
	}
	if _, ok := resumed.Next(); ok {
		t.Fatal("range continues after exhaustion")
	}
}

func TestRangeRejectsInvalidInputAndCursor(t *testing.T) {
	if _, err := NewRange("2001:db8::/64", testKey()); err == nil {
		t.Fatal("expected IPv6 range to be rejected")
	}
	r, err := NewRange("198.51.100.0/24", testKey())
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Restore(257); err == nil {
		t.Fatal("expected out-of-range cursor to be rejected")
	}
}

func testKey() [32]byte {
	return [32]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
}
