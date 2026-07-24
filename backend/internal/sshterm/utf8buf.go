package sshterm

import "unicode/utf8"

// UTF8Buffer accumulates byte chunks and emits only complete UTF-8 sequences.
// SSH PTY reads can split multi-byte characters (e.g. Turkish ğ/ş/ı) across
// packet boundaries; converting each raw chunk to string independently would
// produce invalid UTF-8 and JSON would replace them with U+FFFD (�).
type UTF8Buffer struct {
	pending []byte
}

// Take appends p and returns a string of all complete runes so far.
// Incomplete trailing bytes stay buffered for the next Take call.
func (b *UTF8Buffer) Take(p []byte) string {
	if len(p) == 0 && len(b.pending) == 0 {
		return ""
	}
	data := append(b.pending, p...)
	complete, rest := splitCompleteUTF8(data)
	if len(rest) == 0 {
		b.pending = nil
	} else {
		b.pending = append([]byte(nil), rest...)
	}
	if len(complete) == 0 {
		return ""
	}
	return string(complete)
}

// Flush returns any remaining bytes (may be incomplete / invalid) and clears the buffer.
func (b *UTF8Buffer) Flush() string {
	if len(b.pending) == 0 {
		return ""
	}
	s := string(b.pending)
	b.pending = nil
	return s
}

// splitCompleteUTF8 returns the longest prefix of p that ends on a complete
// UTF-8 rune boundary, and any incomplete trailing bytes.
func splitCompleteUTF8(p []byte) (complete, rest []byte) {
	if len(p) == 0 {
		return nil, nil
	}
	// Walk back at most 3 bytes (max UTF-8 continuation) looking for a RuneStart
	// whose sequence is still incomplete.
	for i := 0; i < 4 && i < len(p); i++ {
		start := len(p) - 1 - i
		if !utf8.RuneStart(p[start]) {
			continue
		}
		if utf8.FullRune(p[start:]) {
			// Last potential multi-byte sequence is complete → whole buffer is fine.
			return p, nil
		}
		// Incomplete sequence starting at start.
		return p[:start], p[start:]
	}
	return p, nil
}
