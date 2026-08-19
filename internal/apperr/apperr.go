// Package apperr is the canonical Go home of Atlas's shared error
// vocabulary (ADR-0024). The twelve Class constants mirror the public
// REST API v1 ErrorClass enum exactly; every layer — providers, store,
// api, workflow dispatch, run classification — normalizes failures
// into these classes. The package is a leaf: it imports nothing from
// Atlas and no transport library.
package apperr

// Class is one of Atlas's shared error classes. The set mirrors the
// public REST API v1 ErrorClass enum byte-for-byte; adding a class
// means updating the OpenAPI enum, the API status map, and
// dev-plans/provider_contracts.md §5 together.
type Class string

const (
	Internal           Class = "Internal"
	InvalidRequest     Class = "InvalidRequest"
	Unavailable        Class = "Unavailable"
	Unauthorized       Class = "Unauthorized"
	Unsupported        Class = "Unsupported"
	VersionUnsupported Class = "VersionUnsupported"
	NotFound           Class = "NotFound"
	Conflict           Class = "Conflict"
	Unsafe             Class = "Unsafe"
	Partial            Class = "Partial"
	MalformedResponse  Class = "MalformedResponse"
	Timeout            Class = "Timeout"
)

// Error is a classified Atlas failure. It is a value type: construct
// it directly, never through a pointer. Internal is constructed only
// at fallback and failure-classification boundaries, never as a
// deliberate validation rejection — caller-input rejections use
// InvalidRequest.
type Error struct {
	Class   Class
	Message string
}

func (e Error) Error() string {
	if e.Message == "" {
		return string(e.Class)
	}
	return string(e.Class) + ": " + e.Message
}

// LookupClass resolves a class name — as parsed from a fixture error
// envelope or any other serialized form — to a known Class. Unknown
// names are rejected so callers can normalize them.
func LookupClass(name string) (Class, bool) {
	class := Class(name)
	switch class {
	case Internal,
		InvalidRequest,
		Unavailable,
		Unauthorized,
		Unsupported,
		VersionUnsupported,
		NotFound,
		Conflict,
		Unsafe,
		Partial,
		MalformedResponse,
		Timeout:
		return class, true
	}
	return "", false
}
