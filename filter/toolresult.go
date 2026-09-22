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
// struct is only used for locating the two places a payload can hide.
const (
	contentKey    = "content"
	structuredKey = "structuredContent"
	textKey       = "text"
	typeKey       = "type"
	textType      = "text"
)

// StripToolResult removes fields from an MCP tool-call result, and returns
// the filtered envelope plus how many locations were actually removed.
//
// `fields` are authored against the *tool's own record* — `$.hashed_password`,
// exactly as docs/CONFIG.md and config.example.toml show — not against the
// JSON-RPC envelope the result arrives in. That envelope can carry the same
// record twice: once as `structuredContent`, and once serialized into a
// `text` content block, which the MCP spec recommends and the Go SDK adds
// automatically whenever a typed tool handler leaves Content unset. Both
// copies are filtered; removing only one leaks the other, which is exactly
// the defect this function exists to fix (see ADR-0015).
//
// The returned count lets the caller notice a filter policy that matched
// the call but removed nothing — usually a path that no longer matches the
// tool's shape, which would otherwise fail silently.
//
// Filtering is enforced or the call fails: a text block that is not a JSON
// document cannot have field paths applied to it, so when `fields` is
// non-empty that returns ErrUnenforceable rather than a response nobody
// checked. With no fields in force there is nothing to enforce and any
// content passes through untouched.
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

	out, err := json.Marshal(envelope)
	if err != nil {
		return nil, 0, fmt.Errorf("filter: marshal filtered tool result: %w", err)
	}
	return out, removed, nil
}

// stripContent filters every text block whose body is a JSON document.
func stripContent(content json.RawMessage, fields []string) (json.RawMessage, int, error) {
	var blocks []map[string]json.RawMessage
	if err := json.Unmarshal(content, &blocks); err != nil {
		return nil, 0, fmt.Errorf("filter: unmarshal tool result content: %w", err)
	}

	removed := 0
	for _, block := range blocks {
		if !isTextBlock(block) {
			// Image, audio, an embedded resource: no JSON fields to
			// strip, so nothing to enforce against it.
			continue
		}
		var text string
		if err := json.Unmarshal(block[textKey], &text); err != nil {
			return nil, 0, fmt.Errorf("%w: unreadable text content", ErrUnenforceable)
		}
		if !json.Valid([]byte(text)) {
			return nil, 0, fmt.Errorf("%w: a filter applies to this call but the tool returned free-form text", ErrUnenforceable)
		}
		filtered, n, err := stripDocument(json.RawMessage(text), fields)
		if err != nil {
			return nil, 0, err
		}
		encoded, err := json.Marshal(string(filtered))
		if err != nil {
			return nil, 0, fmt.Errorf("filter: re-encode filtered text content: %w", err)
		}
		block[textKey] = encoded
		removed += n
	}

	out, err := json.Marshal(blocks)
	if err != nil {
		return nil, 0, fmt.Errorf("filter: marshal filtered content: %w", err)
	}
	return out, removed, nil
}

func isTextBlock(block map[string]json.RawMessage) bool {
	raw, ok := block[typeKey]
	if !ok {
		// A block with no discriminator but a text field is still text;
		// anything else is not ours to touch.
		_, hasText := block[textKey]
		return hasText
	}
	var kind string
	if err := json.Unmarshal(raw, &kind); err != nil {
		return false
	}
	return kind == textType
}

func isJSONNull(raw json.RawMessage) bool {
	return string(raw) == "null"
}
