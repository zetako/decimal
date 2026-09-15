package decimal

import (
	"bytes"
)

// MarshalJSON implements json.Marshaler. It writes the canonical representation
// as a bare JSON number, never as a quoted string, so a Decimal round-trips
// through encoding/json without the caller having to convert it:
//
//	d, _ := decimal.Parse("1.50")
//	b, _ := json.Marshal(d) // 1.5
//
// The result is exact: any JSON decoder that respects the number grammar, as
// encoding/json does with json.Number, reads back the same value, no matter how
// many digits it carries or how large its scale is.
func (d Decimal) MarshalJSON() ([]byte, error) {
	var buf [maxStringLen]byte
	start := encodeDecimal(&buf, d)

	// The JSON encoder validates the result anyway, so the error is unreachable.
	// Building the number directly also avoids the HTML escaping and the quoting
	// that a string encode would apply.
	b := make([]byte, len(buf)-start)
	copy(b, buf[start:])
	return b, nil
}

// UnmarshalJSON implements json.Unmarshaler. It accepts a bare JSON number,
// including one written with an exponent, and it accepts null (which yields the
// canonical zero).
//
// A quoted string is rejected with ErrSyntax. The rule is deliberate: this type
// writes numbers and only numbers, so accepting the string form would let a
// client believe that "12.5" and 12.5 are interchangeable on the wire when the
// producer is expected to emit the former. Use UnmarshalText when the input
// genuinely is text.
//
// The bytes are decoded directly. Parsing never goes through float64, so no
// precision is lost on the way in.
func (d *Decimal) UnmarshalJSON(b []byte) error {
	if d == nil {
		return errSyntax(string(b), "cannot unmarshal into a nil *Decimal")
	}

	// 1. Handle null, which encoding/json also passes through to UnmarshalJSON.
	if bytes.Equal(b, []byte("null")) {
		*d = Decimal{}
		return nil
	}

	// 2. Reject the quoted string form with a message that explains the rule.
	if len(b) > 0 && b[0] == '"' {
		return errSyntax(string(b), "quoted strings are not accepted, a bare JSON number is required")
	}

	// 3. Parse the number. The conversion to string does not copy, and parse
	// re-reads the same bytes from a plain string.
	v, err := parseString(string(b))
	if err != nil {
		return err
	}
	*d = v
	return nil
}

// MarshalText implements encoding.TextMarshaler. The output is identical to
// String(), that is, canonical plain notation without an exponent.
func (d Decimal) MarshalText() ([]byte, error) {
	var buf [maxStringLen]byte
	start := encodeDecimal(&buf, d)
	return append([]byte(nil), buf[start:]...), nil
}

// UnmarshalText implements encoding.TextUnmarshaler using the same grammar as
// Parse. Empty input is rejected with ErrEmptyString as required by
// encoding.TextUnmarshaler; use UnmarshalJSON to accept null, or assign
// decimal.Zero() directly.
func (d *Decimal) UnmarshalText(b []byte) error {
	if len(b) == 0 {
		return ErrEmptyString
	}
	v, err := parseString(string(b))
	if err != nil {
		return err
	}
	*d = v
	return nil
}
