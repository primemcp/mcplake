// Package filter strips response fields identified by the Policy Engine
// (router.Engine.FieldsToRemove) from a downstream MCP's JSON tool
// response. See docs/architecture/decisions/0004-unified-policy-engine-for-access-and-filtering.md.
package filter

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/theory/jsonpath"
	"github.com/theory/jsonpath/spec"
)

// Strip removes the given field paths (JSONPath expressions, e.g. "$.email"
// or "$.items[*].secret") from response, a raw JSON tool-call response
// body, and returns the result. The output is always valid JSON.
//
// A field path that doesn't resolve to anything in this particular response
// is a no-op, not an error — DropFields are authored once against a tool's
// general response shape and not every response necessarily contains every
// optional field.
//
// Only object-field removal is supported: a path that resolves to a bare
// array element (rather than a key within an object) is left in place,
// since DropFields names *fields*, not array positions.
func Strip(response json.RawMessage, fields []string) (json.RawMessage, error) {
	out, _, err := stripDocument(response, fields)
	return out, err
}

// stripDocument is Strip plus the count of locations actually removed. The
// count is what lets StripToolResult tell "this filter removed nothing"
// from "this filter removed something", so a policy whose paths no longer
// match the tool's shape can be surfaced instead of failing silently.
//
// Decoding preserves every number's exact source digits (json.Number)
// rather than collapsing it through float64: a downstream MCP legitimately
// returns 64-bit ids, nanosecond timestamps and monetary integers, and a
// float64 round-trip silently corrupts any of those once they cross 2^53 —
// on a response that may not even contain the field being filtered.
// Marshaling turns escapeHTML off for the same reason: an untouched string
// containing "&"/"<"/">" (a URL query, a code sample) must come back
// byte-for-byte, not rewritten to & etc., just because some unrelated
// field elsewhere in the same document was filtered.
func stripDocument(response json.RawMessage, fields []string) (json.RawMessage, int, error) {
	if len(fields) == 0 {
		return response, 0, nil
	}

	doc, err := decodePreservingNumbers(response)
	if err != nil {
		return nil, 0, fmt.Errorf("filter: unmarshal response: %w", err)
	}

	removed := 0
	for _, field := range fields {
		path, err := jsonpath.Parse(field)
		if err != nil {
			return nil, 0, fmt.Errorf("filter: parse field path %q: %w", field, err)
		}
		for _, located := range path.SelectLocated(doc) {
			if deleteField(doc, located.Path) {
				removed++
			}
		}
	}

	out, err := marshalWithoutHTMLEscaping(doc)
	if err != nil {
		return nil, 0, fmt.Errorf("filter: marshal filtered response: %w", err)
	}
	return out, removed, nil
}

// decodePreservingNumbers unmarshals into `any` the way json.Unmarshal
// would, except every JSON number decodes as json.Number (its exact source
// text) instead of float64. jsonpath's traversal only cares about map keys
// and array indices, never about a leaf's concrete scalar type, so this is
// a drop-in replacement for the document walked by SelectLocated/deleteField.
func decodePreservingNumbers(raw json.RawMessage) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var doc any
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	return doc, nil
}

// marshalWithoutHTMLEscaping is json.Marshal with HTML escaping disabled —
// see stripDocument's doc comment for why an untouched string must survive
// this round trip unchanged. json.Encoder.Encode appends a trailing
// newline that json.Marshal does not; it is trimmed here so every caller
// keeps getting exactly what json.Marshal would have returned, minus the
// escaping.
func marshalWithoutHTMLEscaping(v any) (json.RawMessage, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// deleteField removes the value at path from doc by walking to its parent
// container and deleting the final segment, and reports whether anything
// was actually deleted. doc's maps are mutated through their reference
// semantics, so the document itself needs no reassignment.
func deleteField(doc any, path spec.NormalizedPath) bool {
	if len(path) == 0 {
		return false // can't delete the root itself
	}

	parent := doc
	for _, sel := range path[:len(path)-1] {
		switch s := sel.(type) {
		case spec.Name:
			m, ok := parent.(map[string]any)
			if !ok {
				return false
			}
			parent = m[string(s)]
		case spec.Index:
			list, ok := parent.([]any)
			if !ok || int(s) < 0 || int(s) >= len(list) {
				return false
			}
			parent = list[int(s)]
		default:
			return false
		}
	}

	if name, ok := path[len(path)-1].(spec.Name); ok {
		if m, ok := parent.(map[string]any); ok {
			if _, present := m[string(name)]; present {
				delete(m, string(name))
				return true
			}
		}
	}
	return false
	// spec.Index as the final segment (deleting a bare array element) is
	// intentionally unsupported — see the Strip doc comment.
}
