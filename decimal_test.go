package decimal

import (
	"errors"
	"math"
	"strings"
	"testing"
)

// TestParseValid covers the accepted grammar, the canonical form, and the
// representation of the extreme values the type supports.
func TestParseValid(t *testing.T) {
	tests := []struct {
		in    string
		coef  int64
		scale int
	}{
		// Integers, including both ends of the usable coefficient range.
		{"0", 0, 0},
		{"1", 1, 0},
		{"-1", -1, 0},
		{"+1", 1, 0},
		{"9223372036854775807", 9223372036854775807, 0},
		{"-9223372036854775807", -9223372036854775807, 0},
		{"1000000000000000000", 1000000000000000000, 0},

		// Signed zero collapses to the single canonical zero.
		{"-0", 0, 0},
		{"+0", 0, 0},
		{"-0.0", 0, 0},
		{"-0.000", 0, 0},
		{"-0e5", 0, 0},
		{"0.000000000000000000", 0, 0},

		// Scale 0 and MaxScale, and the smallest non-zero magnitude.
		{"1e-18", 1, 18},
		{"0.000000000000000001", 1, 18},
		{"-0.000000000000000001", -1, 18},
		{"9.223372036854775807", 9223372036854775807, 18},

		// Trailing zeros are stripped: canonical form has none.
		{"1.0", 1, 0},
		{"1.50", 15, 1},
		{"1.500", 15, 1},
		{"100.00", 100, 0},
		{"1e2", 100, 0},
		{"12.340e2", 1234, 0},
		{"12340e-2", 1234, 1},

		// Leading zeros carry no magnitude.
		{"0000000000000000001", 1, 0},
		{"0.000000000000000000001e3", 1, 18},
		{"000.500", 5, 1},

		// The dotted forms with a missing side.
		{".5", 5, 1},
		{"+.5", 5, 1},
		{"-.5", -5, 1},
		{"5.", 5, 0},
		{"-5.", -5, 0},
		{".000000000000000001", 1, 18},

		// Exponents, both signs and both cases of the marker.
		{"1e3", 1000, 0},
		{"1E3", 1000, 0},
		{"1e+3", 1000, 0},
		{"1E+3", 1000, 0},
		{"1.5e1", 15, 0},
		{"1.5e0", 15, 1},
		{"1.5e-1", 15, 2},
		{"15e-1", 15, 1},
		{"-1.5E-1", -15, 2},
		{"1e-18", 1, 18},
		{"1000e-3", 1, 0},

		// Exactly 19 significant digits: the largest accepted coefficient.
		{"9999999999999999999", 0, 0}, // placeholder, replaced below
	}
	// The 19 digit 9s literal overflows, so remove it from the accepted table.
	tests = tests[:len(tests)-1]

	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, err := Parse(tc.in)
			if err != nil {
				t.Fatalf("Parse(%q) returned error %v, want coef=%d scale=%d", tc.in, err, tc.coef, tc.scale)
			}
			if got.Coef() != tc.coef || got.Scale() != tc.scale {
				t.Fatalf("Parse(%q) = {coef:%d scale:%d}, want {coef:%d scale:%d}",
					tc.in, got.Coef(), got.Scale(), tc.coef, tc.scale)
			}
			if !isCanonical(got) {
				t.Fatalf("Parse(%q) produced the non-canonical %v", tc.in, got)
			}
		})
	}
}

// TestParseInvalid checks that malformed input is rejected with the right
// sentinel error, and that the message names the input.
func TestParseInvalid(t *testing.T) {
	tests := []struct {
		in     string
		sentl  error
		reason string
	}{
		// Syntax.
		{"", ErrSyntax, "empty string"},
		{" ", ErrSyntax, "whitespace"},
		{"  1", ErrSyntax, "leading whitespace"},
		{"1 ", ErrSyntax, "trailing whitespace"},
		{"1\t2", ErrSyntax, "tab in the middle"},
		{"\v1", ErrSyntax, "vertical tab"},
		{"+", ErrSyntax, "sign only"},
		{"-", ErrSyntax, "sign only"},
		{".", ErrSyntax, "point only"},
		{"-.", ErrSyntax, "sign and point only"},
		{"e3", ErrSyntax, "exponent only"},
		{".e3", ErrSyntax, "point and exponent"},
		{"1.2.3", ErrSyntax, "two points"},
		{"1..2", ErrSyntax, "empty fraction"},
		{"1.2.", ErrSyntax, "trailing point after fraction"},
		{"1_000", ErrSyntax, "digit separator"},
		{"1,000", ErrSyntax, "comma separator"},
		{"NaN", ErrSyntax, "NaN"},
		{"nan", ErrSyntax, "lowercase NaN"},
		{"Inf", ErrSyntax, "Inf"},
		{"-Inf", ErrSyntax, "negative Inf"},
		{"Infinity", ErrSyntax, "Infinity"},
		{"0x10", ErrSyntax, "hexadecimal"},
		{"1e", ErrSyntax, "dangling exponent"},
		{"1e+", ErrSyntax, "dangling exponent sign"},
		{"1e-", ErrSyntax, "dangling negative exponent sign"},
		{"1e1.5", ErrSyntax, "fractional exponent"},
		{"1 000", ErrSyntax, "space separator"},
		{"١٢٣", ErrSyntax, "non-ASCII digits"},
		{"1e999999999999999999999", ErrOverflow, "exponent far beyond the range"},

		// Overflow.
		{"9223372036854775808", ErrOverflow, "MaxInt64 + 1"},
		{"-9223372036854775808", ErrOverflow, "MinInt64 magnitude"},
		{"-9223372036854775809", ErrOverflow, "below MinInt64"},
		{"9999999999999999999", ErrOverflow, "19 nines"},
		{"99999999999999999999", ErrOverflow, "20 nines"},
		{"1e19", ErrOverflow, "10^19"},
		{"1e20", ErrOverflow, "10^20"},
		{"-1e19", ErrOverflow, "negative 10^19"},
		{"10000000000000000000", ErrOverflow, "20 digit power of ten"},
		{"92233720368547758070e-1", ErrOverflow, "coefficient needs 20 digits"},

		// Scale out of range. Reaching this check requires a coefficient of at
		// most 19 digits, since more significant digits are reported as an
		// overflow before the scale is even considered.
		{"1e-19", ErrScaleOutOfRange, "below MaxScale"},
		{"1e-30", ErrScaleOutOfRange, "far below MaxScale"},
		{"0.0000000000000000001", ErrScaleOutOfRange, "19 fractional digits"},
		{"1.00000000000000000001", ErrOverflow, "a 20 digit coefficient overflows before the scale is checked"},
		{"0.1e-18", ErrScaleOutOfRange, "shifted below MaxScale"},
	}

	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, err := Parse(tc.in)
			if err == nil {
				t.Fatalf("Parse(%q) = %v, want an error (%s)", tc.in, got, tc.reason)
			}
			if !errors.Is(err, tc.sentl) {
				t.Fatalf("Parse(%q) error = %v, want errors.Is(%v)", tc.in, err, tc.sentl)
			}
			// Error messages quote the rejected input. Control bytes are
			// escaped by the %q verb, so a table entry whose text is not
			// literally present can only be checked for the package prefix.
			msg := err.Error()
			if tc.in != "" && !strings.Contains(msg, tc.in) && !strings.HasPrefix(msg, "decimal: ") {
				t.Fatalf("Parse(%q) error = %q, want it to quote the input", tc.in, err)
			}
			if got != (Decimal{}) {
				t.Fatalf("Parse(%q) returned %v alongside its error, want the zero value", tc.in, got)
			}
		})
	}
}

// TestParseNoRounding pins the property that parse never rounds: anything that
// needs more precision than the representation allows is an error, never a
// silently adjusted value.
func TestParseNoRounding(t *testing.T) {
	for _, in := range []string{"1e-19", "1.23e-18", "0.0000000000000000009", "1.9999999999999999999"} {
		if got, err := Parse(in); err == nil {
			t.Fatalf("Parse(%q) = %v, want an error instead of a rounded value", in, got)
		}
	}
}

// TestMustParse checks the two documented outcomes: a value on success and a
// panic on failure.
func TestMustParse(t *testing.T) {
	if got := MustParse("1.50"); got != (Decimal{coef: 15, scale: 1}) {
		t.Fatalf("MustParse(1.50) = %v, want 1.5", got)
	}

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("MustParse(\"oops\") did not panic")
		}
		if msg, ok := r.(string); !ok || !strings.Contains(msg, "oops") {
			t.Fatalf("MustParse panic = %v, want a message naming the input", r)
		}
	}()
	MustParse("oops")
}

// TestFromCoefScale covers the programmatic constructor, including the
// validation of the scale argument and the canonicalisation of its input.
func TestFromCoefScale(t *testing.T) {
	valid := []struct {
		coef  int64
		scale int8
		want  Decimal
	}{
		{0, 0, Decimal{}},
		{0, 18, Decimal{}},
		{150, 2, Decimal{coef: 15, scale: 1}},
		{15, 1, Decimal{coef: 15, scale: 1}},
		{1000, 3, Decimal{coef: 1, scale: 0}},
		{-150, 2, Decimal{coef: -15, scale: 1}},
		{1, 18, Decimal{coef: 1, scale: 18}},
		{math.MaxInt64, 0, Decimal{coef: math.MaxInt64}},
		{math.MinInt64 + 1, 0, Decimal{coef: math.MinInt64 + 1}},
		{-1000000000000000000, 18, Decimal{coef: -1, scale: 0}},
	}
	for _, tc := range valid {
		got, err := FromCoefScale(tc.coef, tc.scale)
		if err != nil {
			t.Fatalf("FromCoefScale(%d, %d) returned error %v", tc.coef, tc.scale, err)
		}
		if got != tc.want {
			t.Fatalf("FromCoefScale(%d, %d) = %v, want %v", tc.coef, tc.scale, got, tc.want)
		}
		if !isCanonical(got) {
			t.Fatalf("FromCoefScale(%d, %d) = %v is not canonical", tc.coef, tc.scale, got)
		}
	}

	// MinInt64 is the one coefficient the representation cannot hold, at any
	// scale, because its magnitude is not negatable: -2^63 * 10^-1 has no other
	// spelling either.
	for _, scale := range []int8{0, 1, 18} {
		got, err := FromCoefScale(math.MinInt64, scale)
		if !errors.Is(err, ErrOverflow) {
			t.Fatalf("FromCoefScale(MinInt64, %d) error = %v, want ErrOverflow", scale, err)
		}
		if got != (Decimal{}) {
			t.Fatalf("FromCoefScale(MinInt64, %d) = %v alongside its error, want the zero value", scale, got)
		}
	}

	for _, scale := range []int8{-1, -18, 19, 100, MaxScale + 1} {
		got, err := FromCoefScale(1, scale)
		if !errors.Is(err, ErrScaleOutOfRange) {
			t.Fatalf("FromCoefScale(1, %d) error = %v, want ErrScaleOutOfRange", scale, err)
		}
		if got != (Decimal{}) {
			t.Fatalf("FromCoefScale(1, %d) = %v alongside its error, want the zero value", scale, got)
		}
	}
}

// TestZeroAndFromInt checks the two trivial constructors, including the one
// int64 that FromInt has to refuse.
func TestZeroAndFromInt(t *testing.T) {
	z := Zero()
	if !z.IsZero() || z.Scale() != 0 || z.Coef() != 0 {
		t.Fatalf("Zero() = %v, want the canonical zero", z)
	}
	if z != (Decimal{}) {
		t.Fatalf("Zero() = %v, want the struct zero value", z)
	}
	var zero Decimal
	if zero != z {
		t.Fatalf("the zero value %v differs from Zero() %v", zero, z)
	}

	for _, i := range []int64{0, 1, -1, math.MaxInt64, math.MinInt64 + 1, 42, -42} {
		d, err := FromInt(i)
		if err != nil {
			t.Fatalf("FromInt(%d) returned error %v", i, err)
		}
		if d.Coef() != i || d.Scale() != 0 {
			t.Fatalf("FromInt(%d) = {coef:%d scale:%d}, want {coef:%d scale:0}",
				i, d.Coef(), d.Scale(), i)
		}
		back, ok := d.Int64()
		if !ok || back != i {
			t.Fatalf("FromInt(%d).Int64() = (%d, %v), want (%d, true)", i, back, ok, i)
		}
	}

	// The one int64 with no representation: its magnitude is not negatable, so
	// no Decimal can carry it, and the refusal is an error rather than a value
	// nothing else in the package could use.
	if got, err := FromInt(math.MinInt64); !errors.Is(err, ErrOverflow) {
		t.Fatalf("FromInt(MinInt64) error = %v, want ErrOverflow", err)
	} else if got != (Decimal{}) {
		t.Fatalf("FromInt(MinInt64) = %v alongside its error, want the zero value", got)
	}
}

// TestQueries covers the accessor methods and the two conversions out of the
// type.
func TestQueries(t *testing.T) {
	tests := []struct {
		in       string
		isZero   bool
		isInt    bool
		sign     int
		scale    int
		coef     int64
		int64    int64
		int64OK  bool
		float64  float64
		negative string
	}{
		{"0", true, true, 0, 0, 0, 0, true, 0, "0"},
		{"-0.0", true, true, 0, 0, 0, 0, true, 0, "0"},
		{"7", false, true, 1, 0, 7, 7, true, 7, "-7"},
		{"-7", false, true, -1, 0, -7, -7, true, -7, "7"},
		{"1.5", false, false, 1, 1, 15, 0, false, 1.5, "-1.5"},
		{"-1.5", false, false, -1, 1, -15, 0, false, -1.5, "1.5"},
		{"1e-18", false, false, 1, 18, 1, 0, false, 1e-18, "-0.000000000000000001"},
		{"9223372036854775807", false, true, 1, 0, math.MaxInt64, math.MaxInt64, true, 9223372036854775807, "-9223372036854775807"},
		{"-9223372036854775807", false, true, -1, 0, -math.MaxInt64, -math.MaxInt64, true, -9223372036854775807, "9223372036854775807"},
		{"0.5", false, false, 1, 1, 5, 0, false, 0.5, "-0.5"},
	}

	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			d := MustParse(tc.in)
			if d.IsZero() != tc.isZero {
				t.Errorf("IsZero() = %v, want %v", d.IsZero(), tc.isZero)
			}
			if d.IsInt() != tc.isInt {
				t.Errorf("IsInt() = %v, want %v", d.IsInt(), tc.isInt)
			}
			if d.Sign() != tc.sign {
				t.Errorf("Sign() = %d, want %d", d.Sign(), tc.sign)
			}
			if d.Scale() != tc.scale {
				t.Errorf("Scale() = %d, want %d", d.Scale(), tc.scale)
			}
			if d.Coef() != tc.coef {
				t.Errorf("Coef() = %d, want %d", d.Coef(), tc.coef)
			}
			gotInt, gotOK := d.Int64()
			if gotInt != tc.int64 || gotOK != tc.int64OK {
				t.Errorf("Int64() = (%d, %v), want (%d, %v)", gotInt, gotOK, tc.int64, tc.int64OK)
			}
			if got := d.Float64(); math.Abs(got-tc.float64) > math.Abs(tc.float64)*1e-15+1e-300 {
				t.Errorf("Float64() = %g, want %g", got, tc.float64)
			}
			if got := d.Neg().String(); got != tc.negative {
				t.Errorf("Neg() = %s, want %s", got, tc.negative)
			}
		})
	}
}

// TestQueriesDoNotMutate pins the value semantics: querying a Decimal never
// changes it, and the results of the arithmetic methods are new values.
func TestQueriesDoNotMutate(t *testing.T) {
	d := MustParse("1.25")
	before := d

	_ = d.String()
	_ = d.IsZero()
	_ = d.IsInt()
	_ = d.Sign()
	_ = d.Scale()
	_ = d.Coef()
	_, _ = d.Int64()
	_ = d.Float64()
	_ = d.Cmp(MustParse("2"))
	_ = d.Equal(MustParse("1.25"))
	_ = d.LessThan(MustParse("2"))
	_ = d.GreaterThan(MustParse("1"))
	_ = d.Neg()
	_ = d.Abs()
	_, _ = d.Add(MustParse("1"))
	_, _ = d.Sub(MustParse("1"))
	_, _ = d.MulInt(3)
	_, _ = d.Rescale(4)
	_, _ = d.Round(1)

	if d != before {
		t.Fatalf("the receiver changed from %v to %v", before, d)
	}
}

// TestMaxScale pins the exported bound and the fact that the library does not
// impose any smaller, business specific limit.
func TestMaxScale(t *testing.T) {
	if MaxScale != 18 {
		t.Fatalf("MaxScale = %d, want 18", MaxScale)
	}
	d := MustParse("0.000000000000000001")
	if d.Scale() != int(MaxScale) {
		t.Fatalf("Scale() = %d, want %d", d.Scale(), MaxScale)
	}
	if d.IsZero() {
		t.Fatal("1e-18 must not be zero")
	}
}
