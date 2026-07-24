package sshterm

import (
	"testing"
)

func TestUTF8Buffer_turkishAcrossBoundary(t *testing.T) {
	// "çağrı" in UTF-8: ç=c3 a7, a=61, ğ=c4 9f, r=72, ı=c4 b1
	full := []byte("çağrı")
	var b UTF8Buffer

	// Split mid-ğ (after first byte of ğ which is at index 3)
	// "ça" = c3 a7 61, then c4 | 9f 72 c4 b1
	part1 := full[:4] // "ça" + first byte of ğ
	part2 := full[4:]

	s1 := b.Take(part1)
	if s1 != "ça" {
		t.Fatalf("part1: got %q want %q (pending incomplete ğ)", s1, "ça")
	}
	s2 := b.Take(part2)
	if s2 != "ğrı" {
		t.Fatalf("part2: got %q want %q", s2, "ğrı")
	}
	if got := b.Flush(); got != "" {
		t.Fatalf("flush: unexpected %q", got)
	}
}

func TestUTF8Buffer_completeChunks(t *testing.T) {
	var b UTF8Buffer
	if got := b.Take([]byte("merhaba")); got != "merhaba" {
		t.Fatalf("got %q", got)
	}
	if got := b.Take([]byte(" şğüıöç")); got != " şğüıöç" {
		t.Fatalf("got %q", got)
	}
}

func TestUTF8Buffer_empty(t *testing.T) {
	var b UTF8Buffer
	if got := b.Take(nil); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestSplitCompleteUTF8(t *testing.T) {
	// Incomplete leading byte of 2-byte char
	p := []byte{'a', 0xc4}
	c, r := splitCompleteUTF8(p)
	if string(c) != "a" || len(r) != 1 || r[0] != 0xc4 {
		t.Fatalf("got complete=%q rest=%v", c, r)
	}
}
