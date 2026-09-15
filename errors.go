package decimal

import (
	"errors"
	"fmt"
)

// Sentinel errors. Every error returned by this package wraps exactly one of
// them, so failures can be classified with errors.Is:
//
//	d, err := decimal.Parse("1e-30")
//	if errors.Is(err, decimal.ErrScaleOutOfRange) {
//		// the value is representable in principle, but needs scale > MaxScale
//	}
var (
	// ErrSyntax reports input that is not a decimal literal, for example an
	// empty string, surrounding whitespace, underscores, "NaN", "Inf", a second
	// decimal point or a dangling exponent marker.
	ErrSyntax = errors.New("decimal: invalid syntax")

	// ErrOverflow reports a value that is well formed but does not fit in an
	// int64 coefficient, either directly (too many significant digits) or as an
	// intermediate result of an operation.
	ErrOverflow = errors.New("decimal: overflow")

	// ErrScaleOutOfRange reports a decimal exponent outside [0, MaxScale]: more
	// than MaxScale fractional digits, or a magnitude that would require a
	// negative scale.
	ErrScaleOutOfRange = errors.New("decimal: scale out of range")

	// ErrEmptyString reports empty input to UnmarshalText, as required by the
	// encoding.TextUnmarshaler contract. Parse reports the same input as
	// ErrSyntax, since an empty literal is a syntax error there.
	ErrEmptyString = errors.New("decimal: empty string")
)

// errScanType builds an error for a Scan source that is not text. It is not a
// sentinel: database/sql replaces any Scanner error with its own
// ErrScanUnsupported, so callers match on that instead.
func errScanType(src any) error {
	return fmt.Errorf("decimal: cannot scan %T into Decimal, expected string, []byte or nil", src)
}

// errSyntax builds an ErrSyntax that quotes the rejected input.
func errSyntax(input, reason string) error {
	return fmt.Errorf("%w %q: %s", ErrSyntax, input, reason)
}

// errOverflow builds an ErrOverflow, quoting the input when one is available.
// An empty input describes an arithmetic intermediate result instead.
func errOverflow(input, reason string) error {
	if input == "" {
		return fmt.Errorf("%w: %s", ErrOverflow, reason)
	}
	return fmt.Errorf("%w %q: %s", ErrOverflow, input, reason)
}

// errScale builds an ErrScaleOutOfRange, quoting the input when one is
// available. An empty input describes a programmatic call instead.
func errScale(input, reason string) error {
	if input == "" {
		return fmt.Errorf("%w: %s", ErrScaleOutOfRange, reason)
	}
	return fmt.Errorf("%w %q: %s", ErrScaleOutOfRange, input, reason)
}
