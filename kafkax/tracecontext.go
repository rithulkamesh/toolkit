package kafkax

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// Header names for W3C Trace Context, carried verbatim on [Envelope.Headers].
const (
	HeaderTraceParent = "traceparent"
	HeaderTraceState  = "tracestate"
)

// TraceContext is a parsed W3C `traceparent`, plus the opaque `tracestate`
// string. It lets kafkax carry a trace across a broker hop without depending
// on any tracing SDK: a producer injects the current context, a consumer
// extracts it and hands it to its tracer.
type TraceContext struct {
	TraceID  string // 32 lowercase hex, not all-zero
	ParentID string // 16 lowercase hex (the sender's span), not all-zero
	Sampled  bool   // low bit of trace-flags
	State    string // tracestate, passed through untouched
}

// ParseTraceParent parses a `traceparent` header value (version 00).
func ParseTraceParent(s string) (TraceContext, error) {
	parts := strings.Split(strings.TrimSpace(s), "-")
	if len(parts) != 4 {
		return TraceContext{}, fmt.Errorf("kafkax: traceparent %q: want 4 dash-separated fields, got %d", s, len(parts))
	}
	version, traceID, parentID, flags := parts[0], parts[1], parts[2], parts[3]

	if version != "00" {
		return TraceContext{}, fmt.Errorf("kafkax: traceparent version %q unsupported", version)
	}
	if !isHex(traceID, 32) || isZero(traceID) {
		return TraceContext{}, fmt.Errorf("kafkax: traceparent trace-id %q invalid", traceID)
	}
	if !isHex(parentID, 16) || isZero(parentID) {
		return TraceContext{}, fmt.Errorf("kafkax: traceparent parent-id %q invalid", parentID)
	}
	if !isHex(flags, 2) {
		return TraceContext{}, fmt.Errorf("kafkax: traceparent flags %q invalid", flags)
	}
	fb, _ := hex.DecodeString(flags)
	return TraceContext{
		TraceID:  strings.ToLower(traceID),
		ParentID: strings.ToLower(parentID),
		Sampled:  fb[0]&0x01 == 0x01,
	}, nil
}

// TraceParent renders the `traceparent` header value.
func (tc TraceContext) TraceParent() string {
	flags := "00"
	if tc.Sampled {
		flags = "01"
	}
	return "00-" + tc.TraceID + "-" + tc.ParentID + "-" + flags
}

// Valid reports whether tc has a well-formed trace-id and parent-id.
func (tc TraceContext) Valid() bool {
	return isHex(tc.TraceID, 32) && !isZero(tc.TraceID) &&
		isHex(tc.ParentID, 16) && !isZero(tc.ParentID)
}

// Inject writes tc onto e's headers as `traceparent` (and `tracestate` when
// [TraceContext.State] is set).
func (tc TraceContext) Inject(e *Envelope) {
	e.SetHeader(HeaderTraceParent, tc.TraceParent())
	if tc.State != "" {
		e.SetHeader(HeaderTraceState, tc.State)
	}
}

// ExtractTraceContext reads a [TraceContext] from e's headers. ok is false when
// there is no `traceparent` or it does not parse.
func ExtractTraceContext(e Envelope) (tc TraceContext, ok bool) {
	raw := e.Header(HeaderTraceParent)
	if raw == "" {
		return TraceContext{}, false
	}
	tc, err := ParseTraceParent(raw)
	if err != nil {
		return TraceContext{}, false
	}
	tc.State = e.Header(HeaderTraceState)
	return tc, true
}

func isHex(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

func isZero(hexStr string) bool {
	for i := 0; i < len(hexStr); i++ {
		if hexStr[i] != '0' {
			return false
		}
	}
	return true
}
