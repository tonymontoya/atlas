package apperr

import (
	"errors"
	"testing"
)

func TestErrorImplementsErrorWithValueReceiver(t *testing.T) {
	var err error = Error{Class: Conflict, Message: "case is closed"}
	if err.Error() != "Conflict: case is closed" {
		t.Fatalf("Error() = %q, want %q", err.Error(), "Conflict: case is closed")
	}
}

func TestErrorFormatsBareClassWithoutMessage(t *testing.T) {
	err := Error{Class: Unavailable}
	if err.Error() != "Unavailable" {
		t.Fatalf("Error() = %q, want %q", err.Error(), "Unavailable")
	}
}

func TestErrorsAsExtractsError(t *testing.T) {
	wrapped := errors.Join(errors.New("outer"), Error{Class: NotFound, Message: "case not found"})
	var appErr Error
	if !errors.As(wrapped, &appErr) {
		t.Fatal("errors.As failed to extract Error from a wrapped chain")
	}
	if appErr.Class != NotFound {
		t.Fatalf("Class = %q, want %q", appErr.Class, NotFound)
	}
	if appErr.Message != "case not found" {
		t.Fatalf("Message = %q, want %q", appErr.Message, "case not found")
	}
}

func TestLookupClassRoundTripsEveryClass(t *testing.T) {
	classes := []Class{
		Internal,
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
		Timeout,
	}
	for _, class := range classes {
		lookedUp, ok := LookupClass(string(class))
		if !ok {
			t.Fatalf("LookupClass(%q) rejected a known class", class)
		}
		if lookedUp != class {
			t.Fatalf("LookupClass(%q) = %q, want identity", class, lookedUp)
		}
	}
	if len(classes) != 12 {
		t.Fatalf("class set has %d members, want 12 matching the public API enum", len(classes))
	}
}

func TestLookupClassRejectsUnknownName(t *testing.T) {
	if _, ok := LookupClass("NotAClass"); ok {
		t.Fatal("LookupClass accepted an unknown class name")
	}
	if _, ok := LookupClass(""); ok {
		t.Fatal("LookupClass accepted the empty string")
	}
}
