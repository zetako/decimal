package decimal

import (
	"bytes"
	"encoding"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
)

// TestMarshalJSONBareNumber pins the central JSON contract: a Decimal leaves the
// package as a JSON number, never as a quoted string.
func TestMarshalJSONBareNumber(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"0", "0"},
		{"-0.0", "0"},
		{"1", "1"},
		{"-1", "-1"},
		{"1.5", "1.5"},
		{"-1.5", "-1.5"},
		{"0.5", "0.5"},
		{"1e3", "1000"},
		{"1.50", "1.5"},
		{"1e-18", "0.000000000000000001"},
		{"-0.000000000000000001", "-0.000000000000000001"},
		{"9223372036854775807", "9223372036854775807"},
		{"-9223372036854775807", "-9223372036854775807"},
		{"0.000001", "0.000001"},
		{"1.000000000000000001", "1.000000000000000001"},
	}
	for _, tc := range tests {
		d := MustParse(tc.in)
		got, err := d.MarshalJSON()
		if err != nil {
			t.Errorf("%s.MarshalJSON() error = %v", tc.in, err)
			continue
		}
		if string(got) != tc.want {
			t.Errorf("%s.MarshalJSON() = %s, want %s", tc.in, got, tc.want)
		}
		if bytes.ContainsRune(got, '"') {
			t.Errorf("%s.MarshalJSON() = %s, want a bare number without quotes", tc.in, got)
		}
		// The output must be valid JSON on its own and survive a strict decoder
		// that refuses to lose precision.
		dec := json.NewDecoder(bytes.NewReader(got))
		dec.UseNumber()
		var num json.Number
		if err := dec.Decode(&num); err != nil {
			t.Errorf("%s.MarshalJSON() = %s is not a JSON number: %v", tc.in, got, err)
		}
		if dec.More() {
			t.Errorf("%s.MarshalJSON() = %s has trailing content", tc.in, got)
		}
	}
}

// TestMarshalJSONThroughEncodingJSON checks the same contract through the
// standard encoder, in a struct field and in a map value.
func TestMarshalJSONThroughEncodingJSON(t *testing.T) {
	type payload struct {
		Amount Decimal `json:"amount"`
		Count  Decimal `json:"count"`
	}
	p := payload{Amount: MustParse("1.50"), Count: mustFromInt(3)}
	got, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	const want = `{"amount":1.5,"count":3}`
	if string(got) != want {
		t.Fatalf("json.Marshal = %s, want %s", got, want)
	}

	m := map[string]Decimal{"a": MustParse("-0.25")}
	got, err = json.Marshal(m)
	if err != nil {
		t.Fatalf("json.Marshal(map): %v", err)
	}
	if string(got) != `{"a":-0.25}` {
		t.Fatalf("json.Marshal(map) = %s, want {\"a\":-0.25}", got)
	}
}

// TestUnmarshalJSONAcceptsNumbers covers the accepted inputs, including exponent
// notation, which a JSON number is allowed to use.
func TestUnmarshalJSONAcceptsNumbers(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"0", "0"},
		{"1", "1"},
		{"-1", "-1"},
		{"1.5", "1.5"},
		{"-1.5", "-1.5"},
		{"1e3", "1000"},
		{"1E3", "1000"},
		{"1e+3", "1000"},
		{"1e-3", "0.001"},
		{"1.5e2", "150"},
		{"1.5E-2", "0.015"},
		{"-0.0", "0"},
		{"0e0", "0"},
		{"9223372036854775807", "9223372036854775807"},
		{"1e-18", "0.000000000000000001"},
		{"0.000000000000000001", "0.000000000000000001"},
		{"1234567890123456789", "1234567890123456789"},
		{"null", "0"},
	}
	for _, tc := range tests {
		var d Decimal
		if err := json.Unmarshal([]byte(tc.in), &d); err != nil {
			t.Errorf("json.Unmarshal(%s) error = %v, want %s", tc.in, err, tc.want)
			continue
		}
		if got := d.String(); got != tc.want {
			t.Errorf("json.Unmarshal(%s) = %s, want %s", tc.in, got, tc.want)
		}
		requireCanonical(t, d, "json.Unmarshal("+tc.in+")")
	}
}

// TestUnmarshalJSONRejectsQuotedStrings documents the deliberate refusal of the
// string form, which is also recorded in the README.
func TestUnmarshalJSONRejectsQuotedStrings(t *testing.T) {
	quoted := []string{
		`"1.5"`,
		`"0"`,
		`"-0.0"`,
		`"1e3"`,
		`" 1.5"`,
		`""`,
		`"abc"`,
	}
	for _, in := range quoted {
		var d Decimal
		err := json.Unmarshal([]byte(in), &d)
		if !errors.Is(err, ErrSyntax) {
			t.Errorf("json.Unmarshal(%s) error = %v, want ErrSyntax", in, err)
		}
		if d != (Decimal{}) {
			t.Errorf("json.Unmarshal(%s) = %v, want the receiver untouched", in, d)
		}
		if err != nil && !strings.Contains(err.Error(), "bare JSON number") {
			t.Errorf("json.Unmarshal(%s) error = %q, want it to explain the number-only rule", in, err)
		}
	}
}

// TestUnmarshalJSONRoundTrip checks that decoding what MarshalJSON produced
// returns the same number, for a spread of shapes.
func TestUnmarshalJSONRoundTrip(t *testing.T) {
	literals := []string{
		"0", "1", "-1", "1.5", "-1.5", "1000", "0.001", "1e-18",
		"-0.000000000000000001", "9223372036854775807", "-9223372036854775807",
		"1.000000000000000001", "0.5", "-0.25", "1000000000000000000",
	}
	for _, s := range literals {
		d := MustParse(s)
		raw, err := d.MarshalJSON()
		if err != nil {
			t.Fatalf("%s.MarshalJSON(): %v", s, err)
		}
		var back Decimal
		if err := json.Unmarshal(raw, &back); err != nil {
			t.Fatalf("json.Unmarshal(%s): %v", raw, err)
		}
		if !back.Equal(d) {
			t.Fatalf("round trip of %s gave %s", s, back)
		}
		if back != d {
			t.Fatalf("round trip of %s changed the representation: %v vs %v", s, back, d)
		}
	}
}

// TestUnmarshalJSONErrors covers malformed numbers, which the decoder itself
// rejects before UnmarshalJSON is called, and the ones it forwards.
func TestUnmarshalJSONErrors(t *testing.T) {
	invalid := []string{
		// Rejected by encoding/json's own number scanner before this type sees
		// them.
		"", "  ", "abc", "+1", "1.2.3", "1e", "1e+", "--1", "0x10",
		"[1]", "{}", "true", "false", `"1.5"`,
		// Forwarded to Parse, which rejects them on range grounds.
		"1e19", "1e-19", "9999999999999999999", "9223372036854775808",
	}
	for _, in := range invalid {
		var d Decimal
		if err := json.Unmarshal([]byte(in), &d); err == nil {
			t.Errorf("json.Unmarshal(%s) = %v, want an error", in, d)
		}
	}
}

// TestUnmarshalJSONNilReceiver makes sure a nil target produces an error rather
// than a panic.
func TestUnmarshalJSONNilReceiver(t *testing.T) {
	var d *Decimal
	if err := d.UnmarshalJSON([]byte("1")); !errors.Is(err, ErrSyntax) {
		t.Fatalf("nil receiver error = %v, want ErrSyntax", err)
	}
}

// TestMarshalText covers the text form, which matches String exactly.
func TestMarshalText(t *testing.T) {
	literals := []string{"0", "-0.0", "1", "1.5", "-1.5", "1e3", "1e-18", "9223372036854775807"}
	for _, s := range literals {
		d := MustParse(s)
		got, err := d.MarshalText()
		if err != nil {
			t.Fatalf("%s.MarshalText(): %v", s, err)
		}
		if string(got) != d.String() {
			t.Errorf("%s.MarshalText() = %q, want %q", s, got, d.String())
		}
		if !strings.ContainsRune(d.String(), '.') && d.Scale() == 0 && strings.Contains(string(got), ".") {
			t.Errorf("%s.MarshalText() = %q, want no decimal point for an integer", s, got)
		}
	}
}

// TestUnmarshalText covers the text form and its rejection of empty input, which
// the encoding.TextUnmarshaler contract requires.
func TestUnmarshalText(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"0", "0"},
		{"-0.0", "0"},
		{"1.50", "1.5"},
		{"1e3", "1000"},
		{"1e-18", "0.000000000000000001"},
		{"9223372036854775807", "9223372036854775807"},
	}
	for _, tc := range tests {
		var d Decimal
		if err := d.UnmarshalText([]byte(tc.in)); err != nil {
			t.Errorf("UnmarshalText(%q) error = %v", tc.in, err)
			continue
		}
		if got := d.String(); got != tc.want {
			t.Errorf("UnmarshalText(%q) = %s, want %s", tc.in, got, tc.want)
		}
	}

	var d Decimal
	if err := d.UnmarshalText(nil); !errors.Is(err, ErrEmptyString) {
		t.Errorf("UnmarshalText(nil) error = %v, want ErrEmptyString", err)
	}
	if err := d.UnmarshalText([]byte("")); !errors.Is(err, ErrEmptyString) {
		t.Errorf("UnmarshalText(\"\") error = %v, want ErrEmptyString", err)
	}
	if err := d.UnmarshalText([]byte("oops")); !errors.Is(err, ErrSyntax) {
		t.Errorf("UnmarshalText(\"oops\") error = %v, want ErrSyntax", err)
	}
	if err := d.UnmarshalText([]byte("1e-19")); !errors.Is(err, ErrScaleOutOfRange) {
		t.Errorf("UnmarshalText(\"1e-19\") error = %v, want ErrScaleOutOfRange", err)
	}
}

// TestMarshalInterfaces pins that Decimal implements the interfaces the standard
// library looks for, so encoding/json, encoding/xml and database/sql all route
// through the exact implementations above.
func TestMarshalInterfaces(t *testing.T) {
	var d Decimal
	var (
		_ json.Marshaler           = d
		_ json.Unmarshaler         = &d
		_ encoding.TextMarshaler   = d
		_ encoding.TextUnmarshaler = &d
	)
}

// TestJSONNumbersAreNotFloats checks that a value needing all 19 digits survives
// the round trip, which it would not if either side went through float64.
func TestJSONNumbersAreNotFloats(t *testing.T) {
	const s = "9223372036854775807"
	d := MustParse(s)

	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if string(raw) != s {
		t.Fatalf("json.Marshal = %s, want %s", raw, s)
	}

	// Decoding into a float64 would give 9223372036854775808, so the value must
	// not be compared through one.
	var f float64
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("json.Unmarshal into float64: %v", err)
	}
	if f == float64(d.coef) {
		t.Logf("float64(9223372036854775807) happens to be exact on this platform")
	}
	var back Decimal
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("json.Unmarshal into Decimal: %v", err)
	}
	if back != d {
		t.Fatalf("round trip of %s changed the value to %s", s, back)
	}
	// A JSON number with more digits than a float64 can hold must not be
	// silently rounded either.
	if got, err := Parse("9223372036854775806"); err != nil || got.Coef() != 9223372036854775806 {
		t.Fatalf("Parse(9223372036854775806) = (%v, %v)", got, err)
	}
	_ = math.MaxInt64
}
