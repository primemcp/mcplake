package filter

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ErrUnenforceable reports that a filter policy applied to this call but
// the response carries content its field paths cannot reach — free-form
// text rather than a JSON document. The gateway must not return such a
// response: the operator asked for fields to be removed and there is no
// way to know whether they are in there.
var ErrUnenforceable = errors.New("filter: response filtering could not be enforced")

// toolResult is the part of the MCP CallToolResult envelope that matters
// here. Everything else (isError, _meta, anything a future protocol
// revision adds) is carried through untouched via the raw map, so this
// struct is only used for locating the places a payload can hide.
const (
	contentKey    = "content"
	structuredKey = "structuredContent"
	textKey       = "text"
	typeKey       = "type"
	textType      = "text"
	resourceType  = "resource"
	resourceKey   = "resource"
)

// StripToolResult removes fields from an MCP tool-call result, and returns
// the filtered envelope plus how many locations were actually removed.
//
// `fields` are authored against the *tool's own record* — `$.hashed_password`,
// exactly as docs/reference/configuration.rst and config.example.toml show — not against the
// JSON-RPC envelope the result arrives in. That envelope can carry the same
// record more than once:
//   - as `structuredContent`;
//   - serialized into a `text` content block, which the MCP spec
//     recommends and the Go SDK adds automatically whenever a typed tool
//     handler leaves Content unset;
//   - serialized into an embedded resource's `resource.text` (a `type:
//     "resource"` content block) — the protocol's other place for a text
//     payload, e.g. a tool that returns "here's the file you asked for"
//     alongside its structured summary.
//
// Every copy is filtered; removing only some leaks the rest, which is
// exactly the defect this function exists to fix (see ADR-0015).
//
// The returned count lets the caller notice a filter policy that matched
// the call but removed nothing — usually a path that no longer matches the
// tool's shape, which would otherwise fail silently.
//
// Filtering is enforced or the call fails: text (top-level or inside a
// resource) that is not a JSON document cannot have field paths applied to
// it, so when `fields` is non-empty that returns ErrUnenforceable rather
// than a response nobody checked. A resource's binary `blob` is never
// JSON and is always left alone — there's nothing to filter and nothing to
// fail closed over. With no fields in force there is nothing to enforce
// and any content passes through untouched.
func StripToolResult(response json.RawMessage, fields []string) (json.RawMessage, int, error) {
	if len(fields) == 0 {
		return response, 0, nil
	}

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(response, &envelope); err != nil {
		return nil, 0, fmt.Errorf("filter: unmarshal tool result: %w", err)
	}

	removed := 0

	if structured, ok := envelope[structuredKey]; ok && !isJSONNull(structured) {
		filtered, n, err := stripDocument(structured, fields)
		if err != nil {
			return nil, 0, err
		}
		envelope[structuredKey] = filtered
		removed += n
	}

	if content, ok := envelope[contentKey]; ok && !isJSONNull(content) {
		filtered, n, err := stripContent(content, fields)
		if err != nil {
			return nil, 0, err
		}
		envelope[contentKey] = filtered
		removed += n
	}

	out, err := marshalWithoutHTMLEscaping(envelope)
	if err != nil {
		return nil, 0, fmt.Errorf("filter: marshal filtered tool result: %w", err)
	}
	return out, removed, nil
}

// stripContent filters every text block (including the text carried by an
// embedded resource) whose body is a JSON document.
func stripContent(content json.RawMessage, fields []string) (json.RawMessage, int, error) {
	var blocks []map[string]json.RawMessage
	if err := json.Unmarshal(content, &blocks); err != nil {
		return nil, 0, fmt.Errorf("filter: unmarshal tool result content: %w", err)
	}

	removed := 0
	for _, block := range blocks {
		switch blockKind(block) {
		case textType:
			n, err := stripBlockText(block, textKey, fields)
			if err != nil {
				return nil, 0, err
			}
			removed += n
		case resourceType:
			n, err := stripEmbeddedResource(block, fields)
			if err != nil {
				return nil, 0, err
			}
			removed += n
		default:
			// Image, audio, a resource *link* (a URI, not inline content):
			// no JSON fields to strip, so nothing to enforce against it.
		}
	}

	out, err := marshalWithoutHTMLEscaping(blocks)
	if err != nil {
		return nil, 0, fmt.Errorf("filter: marshal filtered content: %w", err)
	}
	return out, removed, nil
}

// stripEmbeddedResource filters `block["resource"]["text"]` in place, the
// same way a top-level text block is filtered. `resource.blob` (binary,
// base64) carries no JSON and is left untouched — it is not a place the
// record can hide, so it is neither filtered nor a reason to fail closed.
func stripEmbeddedResource(block map[string]json.RawMessage, fields []string) (int, error) {
	raw, ok := block[resourceKey]
	if !ok || isJSONNull(raw) {
		return 0, nil
	}
	var resource map[string]json.RawMessage
	if err := json.Unmarshal(raw, &resource); err != nil {
		return 0, fmt.Errorf("%w: unreadable embedded resource", ErrUnenforceable)
	}
	if _, hasText := resource[textKey]; !hasText {
		// blob-only (binary) resource: nothing this filter can reach.
		return 0, nil
	}

	removed, err := stripBlockText(resource, textKey, fields)
	if err != nil {
		return 0, err
	}
	encoded, err := marshalWithoutHTMLEscaping(resource)
	if err != nil {
		return 0, fmt.Errorf("filter: re-encode filtered embedded resource: %w", err)
	}
	block[resourceKey] = encoded
	return removed, nil
}

// stripBlockText decodes container[key] as a string holding a serialized
// JSON document, filters that document, and re-encodes it back into
// container[key] in place. Used for both a top-level text content block
// and an embedded resource's `resource.text`, which have the identical
// shape: a JSON string field carrying another JSON document as text.
func stripBlockText(container map[string]json.RawMessage, key string, fields []string) (int, error) {
	var text string
	if err := json.Unmarshal(container[key], &text); err != nil {
		return 0, fmt.Errorf("%w: unreadable text content", ErrUnenforceable)
	}
	if !json.Valid([]byte(text)) {
		return 0, fmt.Errorf("%w: a filter applies to this call but the tool returned free-form text", ErrUnenforceable)
	}
	filtered, removed, err := stripDocument(json.RawMessage(text), fields)
	if err != nil {
		return 0, err
	}
	// marshalWithoutHTMLEscaping, not json.Marshal: encoding a Go string
	// escapes <, >, & inside its contents by default too, not just via
	// the object-level compact step -- an untouched "&" in the record
	// this string carries must survive re-wrapping unchanged.
	encoded, err := marshalWithoutHTMLEscaping(string(filtered))
	if err != nil {
		return 0, fmt.Errorf("filter: re-encode filtered text content: %w", err)
	}
	container[key] = encoded
	return removed, nil
}

// blockKind returns the content block's discriminator: "text" or
// "resource" for the two shapes this package understands, or "" for
// anything else (including a block with no "type" key, per the wire
// format's own fallback: a bare "text" field with no discriminator is
// still text).
func blockKind(block map[string]json.RawMessage) string {
	raw, ok := block[typeKey]
	if !ok {
		if _, hasText := block[textKey]; hasText {
			return textType
		}
		return ""
	}
	var kind string
	if err := json.Unmarshal(raw, &kind); err != nil {
		return ""
	}
	return kind
}

func isJSONNull(raw json.RawMessage) bool {
	return string(raw) == "null"
}
