// Package canon produces the canonical JSON that room entries are signed over
// (rooms spec §6.1): object keys sorted, no whitespace, no HTML escaping,
// numbers kept exactly as written. Deterministic on every platform.
package canon

import (
	"bytes"
	"encoding/json"
)

// Marshal encodes v canonically. v may be any JSON-encodable value.
func Marshal(v any) ([]byte, error) {
	raw, err := encode(v)
	if err != nil {
		return nil, err
	}
	// Round-trip through a generic value: Go sorts map keys on encode, and
	// json.Number keeps integers beyond 2^53 exact.
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var g any
	if err := dec.Decode(&g); err != nil {
		return nil, err
	}
	return encode(g)
}

func encode(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}
