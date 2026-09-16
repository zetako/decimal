package decimal

import (
	"encoding/json"
	"errors"
	"math"
	"testing"
)

// FuzzParse checks the two properties the parser must never break: it never
// panics, and whatever it accepts round-trips through String unchanged.
func FuzzParse(f *testing.F) {
	seeds := []string{
		"0", "-0.0", "1", "-1", "1.5", ".5", "5.", "1e3", "1E+3", "-1.5e-2",
		"9223372036854775807", "-9223372036854775807", "9223372036854775808",
		"1e-18", "1e-19", "1e19", "0.000000000000000001", "1.000000000000000001",
		"9999999999999999999", "1_000", "", " ", "NaN", "Inf", "1.2.3", "1e", "+",
		"000.500", "0.000000000000000000001e21", "-0", "+0", "1e999999999999999999",
		"0.1", "1000000000000000000", "0.0000000000000000001", "1e+3",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		// 1. Parsing must never panic, whatever the bytes are.
		d, err := Parse(s)
		if err != nil {
			// 2. A rejection must be one of the documented sentinels.
			if !isRangeOrSyntaxError(err) {
				t.Fatalf("Parse(%q) error = %v, want a sentinel wrapped with %%w", s, err)
			}
			return
		}

		// 3. Anything accepted must satisfy every invariant.
		checkInvariants(t, d, "Parse("+s+")")

		// 4. String must be the exact inverse of Parse.
		back, err := Parse(d.String())
		if err != nil {
			t.Fatalf("Parse(%q) succeeded with %v, but Parse(%q) failed: %v", s, d, d.String(), err)
		}
		if back != d {
			t.Fatalf("Parse(%q) = %v but Parse(%q) = %v", s, d, d.String(), back)
		}

		// 5. A decimal literal must never be read as a float: re-parsing the
		// canonical text has to give the identical struct.
		if again := MustParse(d.String()); again != d {
			t.Fatalf("MustParse(%q) = %v, want %v", d.String(), again, d)
		}
	})
}

// FuzzCmpArith checks the arithmetic contract on arbitrary pairs of accepted
// literals: comparisons stay antisymmetric, addition, subtraction and
// multiplication either produce an exact result or report a range error, and
// nothing panics.
func FuzzCmpArith(f *testing.F) {
	seeds := [][2]string{
		{"0", "0"},
		{"1", "-1"},
		{"1.5", "1.50"},
		{"0.5", "0.5"},
		{"9223372036854775807", "1"},
		{"9223372036854775807", "0.000000000000000001"},
		{"-9223372036854775807", "0.000000000000000001"},
		{"1e-18", "-1e-18"},
		{"1e3", "1000"},
		{"0.1", "0.2"},
		{"9223372036854775806", "9223372036854775807"},
		{"-0.0", "0"},
		{"1.5", "0.25"},
		{"0.000000001", "0.000000001"},
		{"1e-9", "1e-10"},
		{"4000000000000000000", "0.5"},
		{"4000000000000000000", "0.25"},
		{"0.0000000002", "0.000000005"},
	}
	for _, s := range seeds {
		f.Add(s[0], s[1])
	}

	f.Fuzz(func(t *testing.T, a, b string) {
		da, err := Parse(a)
		if err != nil {
			return
		}
		db, err := Parse(b)
		if err != nil {
			return
		}
		checkInvariants(t, da, "Parse("+a+")")
		checkInvariants(t, db, "Parse("+b+")")

		// 1. Comparison is a total order: antisymmetric, and consistent with
		// the three convenience predicates.
		ab, ba := da.Cmp(db), db.Cmp(da)
		if ab != -ba {
			t.Fatalf("Cmp(%q, %q) = %d but Cmp(%q, %q) = %d", a, b, ab, b, a, ba)
		}
		if ab < -1 || ab > 1 {
			t.Fatalf("Cmp(%q, %q) = %d, want a value in {-1, 0, 1}", a, b, ab)
		}
		if da.Equal(db) != (ab == 0) || da.LessThan(db) != (ab < 0) || da.GreaterThan(db) != (ab > 0) {
			t.Fatalf("the predicates disagree with Cmp for %q and %q", a, b)
		}

		// 2. Equal values must have equal text, because both are canonical.
		if ab == 0 && da.String() != db.String() {
			t.Fatalf("equal values %q and %q print differently: %q vs %q", a, b, da, db)
		}

		// 3. Addition and subtraction are exact or they fail loudly.
		if sum, err := da.Add(db); err != nil {
			if !isRangeError(err) {
				t.Fatalf("Add(%q, %q) error = %v", a, b, err)
			}
		} else {
			checkInvariants(t, sum, "Add("+a+", "+b+")")
			// Commutativity.
			if other, err := db.Add(da); err != nil || other != sum {
				t.Fatalf("Add is not commutative for %q and %q: %v vs %v (err %v)", a, b, sum, other, err)
			}
			// A zero result is exactly the canonical zero.
			if sum.IsZero() && sum != (Decimal{}) {
				t.Fatalf("Add(%q, %q) produced the non-canonical zero %v", a, b, sum)
			}
		}

		diff, err := da.Sub(db)
		if err != nil {
			if !isRangeError(err) {
				t.Fatalf("Sub(%q, %q) error = %v", a, b, err)
			}
			return
		}
		checkInvariants(t, diff, "Sub("+a+", "+b+")")

		// 4. Subtraction and addition of the negation agree.
		if negSum, err := da.Add(db.Neg()); err == nil && negSum != diff {
			t.Fatalf("Sub(%q, %q) = %v but Add(%q, -%q) = %v", a, b, diff, a, b, negSum)
		}
		// 5. d - d is zero whenever it is representable at all.
		if diffZero, err := da.Sub(da); err != nil {
			t.Fatalf("Sub(%q, %q) failed with %v, but it is always exactly zero", a, a, err)
		} else if !diffZero.IsZero() {
			t.Fatalf("Sub(%q, %q) = %v, want zero", a, a, diffZero)
		}

		// 6. Multiplication is exact or it fails loudly, and it commutes.
		if prod, err := da.Mul(db); err != nil {
			if !isRangeError(err) {
				t.Fatalf("Mul(%q, %q) error = %v", a, b, err)
			}
		} else {
			checkInvariants(t, prod, "Mul("+a+", "+b+")")
			if other, err := db.Mul(da); err != nil || other != prod {
				t.Fatalf("Mul is not commutative for %q and %q: %v vs %v (err %v)", a, b, prod, other, err)
			}
		}

		// 7. Multiplying by an integer agrees with MulInt on the same operand,
		// both on the value and on whether it succeeds at all.
		if prod, err := da.Mul(FromInt(3)); err != nil {
			if viaInt, iErr := da.MulInt(3); iErr == nil {
				t.Fatalf("Mul(%q, 3) failed with %v but MulInt(%q, 3) = %v", a, err, a, viaInt)
			}
		} else if viaInt, iErr := da.MulInt(3); iErr != nil || viaInt != prod {
			t.Fatalf("Mul(%q, 3) = %v but MulInt(%q, 3) = (%v, %v)", a, prod, a, viaInt, iErr)
		}
	})
}

// FuzzRound checks that Round always returns a value at or below the requested
// scale, never panics, and stays within one unit of the input.
func FuzzRound(f *testing.F) {
	seeds := []struct {
		s      string
		target int8
	}{
		{"1.5", 0}, {"2.5", 0}, {"-2.5", 0}, {"0.5", 0}, {"1.25", 1},
		{"0.000000000000000001", 18}, {"9223372036854775807", 0},
		{"1.23456789012345678", 5}, {"-1.23456789012345678", 5}, {"0", 0},
		{"1", 18}, {"-0.05", 1},
	}
	for _, s := range seeds {
		f.Add(s.s, s.target)
	}

	f.Fuzz(func(t *testing.T, s string, target int8) {
		d, err := Parse(s)
		if err != nil {
			return
		}
		got, err := d.Round(target)
		if err != nil {
			if !isRangeError(err) {
				t.Fatalf("Round(%q, %d) error = %v", s, target, err)
			}
			return
		}
		checkInvariants(t, got, "Round("+s+")")

		if target < 0 || target > MaxScale {
			t.Fatalf("Round(%q, %d) succeeded with an out of range target", s, target)
		}
		if got.Scale() > int(target) {
			t.Fatalf("Round(%q, %d) = %v has scale %d", s, target, got, got.Scale())
		}
		// Rounding to a scale at or above the current one is exact.
		if int(target) >= d.Scale() && !got.Equal(d) {
			t.Fatalf("Round(%q, %d) = %v changed an exact conversion", s, target, got)
		}
		// And rounding down cannot move the value by more than half a unit at
		// the target scale.
		if int(target) < d.Scale() {
			if diff, err := got.Sub(d); err == nil {
				if maxStep, err := FromCoefScale(5, target+1); err == nil && diff.Abs().Cmp(maxStep) > 0 {
					t.Fatalf("Round(%q, %d) = %v moved the value too far", s, target, got)
				}
			}
		}
	})
}

// FuzzRescale checks that Rescale never changes a value and never panics.
func FuzzRescale(f *testing.F) {
	f.Add("1.5", int8(3))
	f.Add("1.5", int8(0))
	f.Add("0", int8(18))
	f.Add("9223372036854775807", int8(18))
	f.Add("0.000000000000000001", int8(0))

	f.Fuzz(func(t *testing.T, s string, target int8) {
		d, err := Parse(s)
		if err != nil {
			return
		}
		got, err := d.Rescale(target)
		if err != nil {
			if !isRangeError(err) {
				t.Fatalf("Rescale(%q, %d) error = %v", s, target, err)
			}
			return
		}
		checkInvariants(t, got, "Rescale("+s+")")
		if !got.Equal(d) {
			t.Fatalf("Rescale(%q, %d) = %v changed the value", s, target, got)
		}
		if got.Scale() > int(target) {
			t.Fatalf("Rescale(%q, %d) = %v has scale %d", s, target, got, got.Scale())
		}
	})
}

// FuzzJSON checks that decoding never panics, that a quoted string is always
// refused, and that whatever is accepted marshals back to a bare number that
// decodes to the same value.
func FuzzJSON(f *testing.F) {
	seeds := []string{
		"0", "1", "-1", "1.5", "1e3", "1E+3", "null", `"1.5"`, `""`, "1e-18",
		"9223372036854775807", "1e19", "1e-19", "abc", "", " 1.5 ", "[1]", "{}",
		"1.2.3", "0.000000000000000001", "-0.0", "true",
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, raw []byte) {
		var d Decimal
		err := d.UnmarshalJSON(raw)
		if err != nil {
			return
		}
		checkInvariants(t, d, "UnmarshalJSON")

		// 1. A quoted string must never be accepted.
		if len(raw) > 0 && raw[0] == '"' {
			t.Fatalf("UnmarshalJSON accepted the quoted form %q", raw)
		}

		// 2. Marshalling produces a bare number that decodes back exactly.
		out, err := d.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON of %v: %v", d, err)
		}
		for _, b := range out {
			if b == '"' {
				t.Fatalf("MarshalJSON produced a quoted value: %s", out)
			}
		}
		var back Decimal
		if err := json.Unmarshal(out, &back); err != nil {
			t.Fatalf("json.Unmarshal(%s) failed: %v", out, err)
		}
		if back != d {
			t.Fatalf("JSON round trip of %v through %s gave %v", d, out, back)
		}

		// 3. The text form agrees with the JSON number.
		text, err := d.MarshalText()
		if err != nil {
			t.Fatalf("MarshalText of %v: %v", d, err)
		}
		if string(text) != string(out) {
			t.Fatalf("MarshalText gave %s but MarshalJSON gave %s", text, out)
		}
	})
}

// FuzzScanValue checks the database round trip on arbitrary accepted text.
func FuzzScanValue(f *testing.F) {
	f.Add("1.5")
	f.Add("0")
	f.Add("-9223372036854775807")
	f.Add("0.000000000000000001")
	f.Add("oops")

	f.Fuzz(func(t *testing.T, s string) {
		var d Decimal
		if err := d.Scan(s); err != nil {
			return
		}
		checkInvariants(t, d, "Scan")
		v, err := d.Value()
		if err != nil {
			t.Fatalf("Value of %v: %v", d, err)
		}
		text, ok := v.(string)
		if !ok {
			t.Fatalf("Value returned %#v, want a string", v)
		}
		var back Decimal
		if err := back.Scan(text); err != nil {
			t.Fatalf("Scan(%q) failed: %v", text, err)
		}
		if back != d {
			t.Fatalf("Scan/Value round trip of %v gave %v", d, back)
		}
	})
}

// checkInvariants fails the test when d breaks any documented promise. It is the
// single place the fuzz targets assert the representation contract.
func checkInvariants(t *testing.T, d Decimal, ctx string) {
	t.Helper()
	if !isCanonical(d) {
		t.Fatalf("%s: %v is not canonical", ctx, d)
	}
	if d.scale < 0 || d.scale > MaxScale {
		t.Fatalf("%s: scale %d is outside [0, %d]", ctx, d.scale, MaxScale)
	}
	if d.coef == math.MinInt64 {
		t.Fatalf("%s: coefficient is MinInt64, whose magnitude is not representable", ctx)
	}
	if d.IsZero() && d != (Decimal{}) {
		t.Fatalf("%s: zero is not the canonical zero: %v", ctx, d)
	}
	if d.IsInt() != (d.scale == 0) {
		t.Fatalf("%s: IsInt %v disagrees with scale %d", ctx, d.IsInt(), d.scale)
	}
	if d.Sign() != signOf(d.coef) {
		t.Fatalf("%s: Sign %d disagrees with coefficient %d", ctx, d.Sign(), d.coef)
	}
	if d.String() != (Decimal{coef: d.coef, scale: d.scale}).String() {
		t.Fatalf("%s: String is not a pure function of the value", ctx)
	}
}

func signOf(v int64) int {
	switch {
	case v < 0:
		return -1
	case v > 0:
		return 1
	default:
		return 0
	}
}

// isRangeOrSyntaxError reports whether err wraps one of the sentinels the package
// documents for input rejection.
func isRangeOrSyntaxError(err error) bool {
	return isRangeError(err) || errors.Is(err, ErrSyntax) || errors.Is(err, ErrEmptyString)
}

// isRangeError reports whether err wraps one of the two numeric range sentinels.
func isRangeError(err error) bool {
	return errors.Is(err, ErrOverflow) || errors.Is(err, ErrScaleOutOfRange)
}
