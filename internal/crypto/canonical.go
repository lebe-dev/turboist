package crypto

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// CanonicalJSON encodes v to a deterministic byte form suitable for hashing and
// signing across instances (Federation v1 F0.3, FEDERATION-ARCH §5.2). The
// encoding has:
//
//   - object keys sorted lexicographically at every nesting level,
//   - no insignificant whitespace (compact),
//   - HTML escaping disabled (SetEscapeHTML(false)) so &, <, > survive
//     verbatim — two peers computing a digest over the same value must agree.
//
// NFC string normalization is a documented v1 gap (R17): the same canonicalizer
// must be used on both the signing and verifying sides so the gap is symmetric.
//
// v is first marshalled with the standard library, then re-decoded into the
// generic any tree so map key ordering is fully under our control regardless of
// the input type (struct field order vs map iteration order).
func CanonicalJSON(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("canonical marshal: %w", err)
	}
	var tree any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&tree); err != nil {
		return nil, fmt.Errorf("canonical decode: %w", err)
	}
	var buf bytes.Buffer
	if err := writeCanonical(&buf, tree); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeCanonical(buf *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case map[string]any:
		return writeCanonicalObject(buf, t)
	case []any:
		return writeCanonicalArray(buf, t)
	default:
		return writeCanonicalScalar(buf, v)
	}
}

func writeCanonicalObject(buf *bytes.Buffer, m map[string]any) error {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	buf.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		if err := writeCanonicalScalar(buf, k); err != nil {
			return err
		}
		buf.WriteByte(':')
		if err := writeCanonical(buf, m[k]); err != nil {
			return err
		}
	}
	buf.WriteByte('}')
	return nil
}

func writeCanonicalArray(buf *bytes.Buffer, a []any) error {
	buf.WriteByte('[')
	for i, item := range a {
		if i > 0 {
			buf.WriteByte(',')
		}
		if err := writeCanonical(buf, item); err != nil {
			return err
		}
	}
	buf.WriteByte(']')
	return nil
}

// writeCanonicalScalar encodes a leaf value (string, json.Number, bool, nil)
// with HTML escaping disabled and no trailing newline.
func writeCanonicalScalar(buf *bytes.Buffer, v any) error {
	var sb bytes.Buffer
	enc := json.NewEncoder(&sb)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("canonical scalar: %w", err)
	}
	// json.Encoder appends a trailing newline — strip it.
	out := bytes.TrimRight(sb.Bytes(), "\n")
	buf.Write(out)
	return nil
}
