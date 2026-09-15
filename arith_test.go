package decimal

import (
	"errors"
	"math"
	"strconv"
	"testing"
)

// TestNegAbs covers the two sign operations, including the extreme values of
// the range.
func TestNegAbs(t *testing.T) {
	tests := []struct {
		in, neg, abs string
	}{
		{"0", "0", "0"},
		{"-0.0", "0", "0"},
		{"1", "-1", "1"},
		{"-1", "1", "1"},
		{"1.5", "-1.5", "1.5"},
		{"-1.5", "1.5", "1.5"},
		{"1e-18", "-0.000000000000000001", "0.000000000000000001"},
		{"-1e-18", "0.000000000000000001", "0.000000000000000001"},
		{"9223372036854775807", "-9223372036854775807", "9223372036854775807"},
		{"-9223372036854775807", "9223372036854775807", "9223372036854775807"},
	}
	for _, tc := range tests {
		d := MustParse(tc.in)
		neg := d.Neg()
		abs := d.Abs()
		if got := neg.String(); got != tc.neg {
			t.Errorf("%s.Neg() = %s, want %s", tc.in, got, tc.neg)
		}
		if got := abs.String(); got != tc.abs {
			t.Errorf("%s.Abs() = %s, want %s", tc.in, got, tc.abs)
		}
		requireCanonical(t, neg, tc.in+".Neg()")
		requireCanonical(t, abs, tc.in+".Abs()")
		if !neg.Neg().Equal(d) {
			t.Errorf("%s.Neg().Neg() = %s, want the original %s", tc.in, neg.Neg(), tc.in)
		}
		if abs.Sign() < 0 {
			t.Errorf("%s.Abs() = %s must not be negative", tc.in, abs)
		}
		if !abs.Equal(d.Abs()) {
			t.Errorf("%s.Abs() is inconsistent", tc.in)
		}
	}
}

// TestAddSub covers exact addition and subtraction, the canonicalisation of the
// result, and the alignment step that makes it possible.
func TestAddSub(t *testing.T) {
	tests := []struct {
		a, b string
		sum  string
		diff string
	}{
		{"0", "0", "0", "0"},
		{"1", "2", "3", "-1"},
		{"1", "-2", "-1", "3"},
		{"-1", "-2", "-3", "1"},
		{"1.5", "2", "3.5", "-0.5"},
		{"1.5", "1.5", "3", "0"},
		{"0.5", "0.5", "1", "0"},
		{"1.25", "0.75", "2", "0.5"},
		{"-1.25", "-0.75", "-2", "-0.5"},
		{"1e-18", "1e-18", "0.000000000000000002", "0"},
		{"0.000000000000000001", "-0.000000000000000001", "0", "0.000000000000000002"},
		{"1.000000000000000001", "0.000000000000000001", "1.000000000000000002", "1"},
		{"1", "0.1", "1.1", "0.9"},
		{"0.1", "0.2", "0.3", "-0.1"},
		{"1000", "0.001", "1000.001", "999.999"},
		{"9223372036854775807", "0", "9223372036854775807", "9223372036854775807"},
		{"-9223372036854775807", "0", "-9223372036854775807", "-9223372036854775807"},
		{"0.000000000000000001", "-1", "-0.999999999999999999", "1.000000000000000001"},
	}
	for _, tc := range tests {
		a, b := MustParse(tc.a), MustParse(tc.b)

		sum, err := a.Add(b)
		if err != nil {
			t.Errorf("%s.Add(%s) returned error %v, want %s", tc.a, tc.b, err, tc.sum)
		} else {
			if got := sum.String(); got != tc.sum {
				t.Errorf("%s.Add(%s) = %s, want %s", tc.a, tc.b, got, tc.sum)
			}
			requireCanonical(t, sum, tc.a+"+"+tc.b)
		}

		diff, err := a.Sub(b)
		if err != nil {
			t.Errorf("%s.Sub(%s) returned error %v, want %s", tc.a, tc.b, err, tc.diff)
		} else {
			if got := diff.String(); got != tc.diff {
				t.Errorf("%s.Sub(%s) = %s, want %s", tc.a, tc.b, got, tc.diff)
			}
			requireCanonical(t, diff, tc.a+"-"+tc.b)
		}

		// Subtraction is addition of the negation.
		if diff, err := a.Sub(b); err == nil {
			if neg, err := a.Add(b.Neg()); err == nil && !diff.Equal(neg) {
				t.Errorf("%s.Sub(%s) = %s but %s.Add(%s.Neg()) = %s", tc.a, tc.b, diff, tc.a, tc.b, neg)
			}
		}
	}
}

// TestAddAlignmentOverflow pins requirement 6: aligning the operands is itself
// an overflow site, because stretching the coarser one can exceed the int64
// range even when both operands are perfectly valid.
func TestAddAlignmentOverflow(t *testing.T) {
	tests := []struct {
		a, b string
	}{
		{"9223372036854775807", "0.000000000000000001"},
		{"0.000000000000000001", "9223372036854775807"},
		{"9223372036854775807", "0.1"},
		{"0.1", "9223372036854775807"},
		{"-9223372036854775807", "0.000000000000000001"},
		{"-9223372036854775807", "-0.5"},
		{"1000", "0.000000000000000001"},
		{"0.000000000000000001", "1000"},
	}
	for _, tc := range tests {
		a, b := MustParse(tc.a), MustParse(tc.b)
		got, err := a.Add(b)
		if !errors.Is(err, ErrOverflow) {
			t.Errorf("%s.Add(%s) error = %v, want ErrOverflow", tc.a, tc.b, err)
		}
		if got != (Decimal{}) {
			t.Errorf("%s.Add(%s) = %v alongside its error, want the zero value", tc.a, tc.b, got)
		}
	}
}

// TestAddSumOverflow pins the second overflow site: two aligned coefficients can
// each fit in an int64 and still sum out of range.
func TestAddSumOverflow(t *testing.T) {
	big := MustParse("9223372036854775807")
	one := FromInt(1)
	if _, err := big.Add(one); !errors.Is(err, ErrOverflow) {
		t.Errorf("MaxInt64 + 1 error = %v, want ErrOverflow", err)
	}
	if _, err := big.Sub(FromInt(-1)); !errors.Is(err, ErrOverflow) {
		t.Errorf("MaxInt64 - (-1) error = %v, want ErrOverflow", err)
	}
	negBig := MustParse("-9223372036854775807")
	// -MaxInt64 + (-1) is exactly MinInt64, which is the one int64 value the
	// representation deliberately does not carry, since the magnitude of a
	// coefficient must be negatable.
	if got, err := negBig.Add(FromInt(-1)); !errors.Is(err, ErrOverflow) {
		t.Errorf("-MaxInt64 + (-1) = (%v, %v), want ErrOverflow", got, err)
	}
	if got, err := negBig.Add(negBig); !errors.Is(err, ErrOverflow) {
		t.Errorf("-MaxInt64 + (-MaxInt64) = (%v, %v), want ErrOverflow", got, err)
	}
	if got, err := MustParse("-9223372036854775806").Add(FromInt(-1)); err != nil ||
		got.String() != "-9223372036854775807" {
		t.Errorf("-(MaxInt64 - 1) + (-1) = (%v, %v), want -9223372036854775807", got, err)
	}
	// Just inside the bound.
	if got, err := MustParse("9223372036854775806").Add(one); err != nil || got.String() != "9223372036854775807" {
		t.Errorf("MaxInt64 - 1 + 1 = (%v, %v), want 9223372036854775807", got, err)
	}
	// MaxInt64 - MaxInt64 is exactly zero, and zero is canonical.
	if got, err := big.Sub(big); err != nil || !got.IsZero() || got.String() != "0" {
		t.Errorf("MaxInt64 - MaxInt64 = (%v, %v), want 0", got, err)
	}
	// MaxInt64 - (-MaxInt64) is 2*MaxInt64, far outside the range.
	if got, err := big.Sub(negBig); !errors.Is(err, ErrOverflow) {
		t.Errorf("MaxInt64 - (-MaxInt64) = (%v, %v), want ErrOverflow", got, err)
	}
	if got, err := negBig.Sub(big); !errors.Is(err, ErrOverflow) {
		t.Errorf("-MaxInt64 - MaxInt64 = (%v, %v), want ErrOverflow", got, err)
	}
}

// TestMulInt covers exact multiplication by an integer, including the identity
// cases and the overflow boundary.
func TestMulInt(t *testing.T) {
	tests := []struct {
		in  string
		i   int64
		out string
	}{
		{"0", 5, "0"},
		{"5", 0, "0"},
		{"-5", 0, "0"},
		{"1.5", 0, "0"},
		{"0", 0, "0"},
		{"5", 1, "5"},
		{"-5", 1, "-5"},
		{"1.5", 1, "1.5"},
		{"1.5", 2, "3"},
		{"1.5", 3, "4.5"},
		{"1.5", -2, "-3"},
		{"-1.5", -2, "3"},
		{"0.1", 3, "0.3"},
		{"0.000000000000000001", 3, "0.000000000000000003"},
		{"0.000000000000000001", -1, "-0.000000000000000001"},
		{"123456789", 1000, "123456789000"},
		{"9223372036854775807", 1, "9223372036854775807"},
		{"-9223372036854775807", 1, "-9223372036854775807"},
		{"9223372036854775807", -1, "-9223372036854775807"},
		{"0.000000000000000001", 9223372036854775807, "9.223372036854775807"},
	}
	for _, tc := range tests {
		d := MustParse(tc.in)
		got, err := d.MulInt(tc.i)
		if err != nil {
			t.Errorf("%s.MulInt(%d) returned error %v, want %s", tc.in, tc.i, err, tc.out)
			continue
		}
		if s := got.String(); s != tc.out {
			t.Errorf("%s.MulInt(%d) = %s, want %s", tc.in, tc.i, s, tc.out)
		}
		requireCanonical(t, got, tc.in+"*int")
	}
}

// TestMulIntOverflow pins that multiplication reports overflow instead of
// wrapping.
func TestMulIntOverflow(t *testing.T) {
	big := MustParse("9223372036854775807")
	for _, i := range []int64{2, -2, 3, 10, math.MaxInt64, math.MinInt64} {
		got, err := big.MulInt(i)
		if !errors.Is(err, ErrOverflow) {
			t.Errorf("MaxInt64.MulInt(%d) error = %v, want ErrOverflow", i, err)
		}
		if got != (Decimal{}) {
			t.Errorf("MaxInt64.MulInt(%d) = %v alongside its error, want the zero value", i, got)
		}
	}
	// The largest accepted product, and the first rejected one.
	if got, err := MustParse("4611686018427387903").MulInt(2); err != nil || got.String() != "9223372036854775806" {
		t.Errorf("boundary product = (%v, %v), want 9223372036854775806", got, err)
	}
	if _, err := MustParse("4611686018427387904").MulInt(2); !errors.Is(err, ErrOverflow) {
		t.Errorf("first rejected product error = %v, want ErrOverflow", err)
	}
}

// TestRescale covers the strict, precision preserving conversion.
func TestRescale(t *testing.T) {
	valid := []struct {
		in     string
		target int8
		out    string
	}{
		{"0", 0, "0"},
		{"0", 18, "0"}, // zero stays canonical at any target scale
		{"1", 0, "1"},
		{"1", 1, "1"},
		{"1", 18, "1"},
		{"1.5", 1, "1.5"},
		{"1.5", 3, "1.5"},
		{"1.5", 18, "1.5"},
		{"0.000000000000000001", 18, "0.000000000000000001"},
		{"-1.5", 4, "-1.5"},
		{"-0.000000000000000001", 18, "-0.000000000000000001"},
	}
	for _, tc := range valid {
		d := MustParse(tc.in)
		got, err := d.Rescale(tc.target)
		if err != nil {
			t.Errorf("%s.Rescale(%d) returned error %v, want %s", tc.in, tc.target, err, tc.out)
			continue
		}
		if s := got.String(); s != tc.out {
			t.Errorf("%s.Rescale(%d) = %s, want %s", tc.in, tc.target, s, tc.out)
		}
		requireCanonical(t, got, tc.in+".Rescale")
		if !got.Equal(d) {
			t.Errorf("%s.Rescale(%d) changed the value to %s", tc.in, tc.target, got)
		}
	}
}

// TestRescaleErrors covers both failure modes of Rescale: a target outside the
// supported range, scaling up that does not fit, and scaling down that would
// drop a digit.
func TestRescaleErrors(t *testing.T) {
	d := MustParse("1.5")

	for _, target := range []int8{-1, -18, 19, 100} {
		got, err := d.Rescale(target)
		if !errors.Is(err, ErrScaleOutOfRange) {
			t.Errorf("Rescale(%d) error = %v, want ErrScaleOutOfRange", target, err)
		}
		if got != (Decimal{}) {
			t.Errorf("Rescale(%d) = %v alongside its error, want the zero value", target, got)
		}
	}

	// Scaling down never loses precision: 1.5 is 15 at scale 1, and 15 is not
	// divisible by any positive power of ten.
	if got, err := d.Rescale(0); !errors.Is(err, ErrOverflow) {
		t.Errorf("1.5.Rescale(0) = (%v, %v), want ErrOverflow", got, err)
	}
	if got, err := MustParse("12.34").Rescale(1); !errors.Is(err, ErrOverflow) {
		t.Errorf("12.34.Rescale(1) = (%v, %v), want ErrOverflow", got, err)
	}
	// ... while an exact reduction is allowed.
	if got, err := MustParse("1.50").Rescale(1); err != nil || got.String() != "1.5" {
		t.Errorf("1.50.Rescale(1) = (%v, %v), want 1.5", got, err)
	}

	// Scaling up is bounded by the coefficient range.
	big := MustParse("9223372036854775807")
	if got, err := big.Rescale(1); !errors.Is(err, ErrOverflow) {
		t.Errorf("MaxInt64.Rescale(1) = (%v, %v), want ErrOverflow", got, err)
	}
	if got, err := big.Rescale(18); !errors.Is(err, ErrOverflow) {
		t.Errorf("MaxInt64.Rescale(18) = (%v, %v), want ErrOverflow", got, err)
	}
	if got, err := MustParse("9").Rescale(18); !errors.Is(err, ErrOverflow) {
		t.Errorf("9.Rescale(18) = (%v, %v), want ErrOverflow", got, err)
	}
	if got, err := MustParse("0.9").Rescale(18); err != nil {
		t.Errorf("0.9.Rescale(18) error = %v, want the exact 0.9", err)
	} else if !got.Equal(MustParse("0.9")) {
		t.Errorf("0.9.Rescale(18) = %s, want 0.9", got)
	}
}

// TestRound covers half-to-even rounding, the one place where the package is
// allowed to lose precision, and it does so only on request.
func TestRound(t *testing.T) {
	tests := []struct {
		in     string
		target int8
		out    string
	}{
		// No-op when the target is at or above the current scale.
		{"1.5", 1, "1.5"},
		{"1.5", 4, "1.5"},
		{"1.5", 18, "1.5"},
		{"0", 0, "0"},
		{"123", 0, "123"},

		// Half-to-even at the midpoint, both parities and both signs.
		{"2.5", 0, "2"},
		{"3.5", 0, "4"},
		{"1.5", 0, "2"},
		{"0.5", 0, "0"},
		{"-2.5", 0, "-2"},
		{"-3.5", 0, "-4"},
		{"-1.5", 0, "-2"},
		{"-0.5", 0, "0"},
		{"0.25", 1, "0.2"},
		{"0.35", 1, "0.4"},
		{"0.45", 1, "0.4"},
		{"0.55", 1, "0.6"},
		{"-0.25", 1, "-0.2"},
		{"-0.35", 1, "-0.4"},

		// Below and above the midpoint.
		{"1.4", 0, "1"},
		{"1.6", 0, "2"},
		{"-1.4", 0, "-1"},
		{"-1.6", 0, "-2"},
		{"0.4", 0, "0"},
		{"1.234", 2, "1.23"},
		{"1.235", 2, "1.24"},
		{"1.236", 2, "1.24"},
		{"1.245", 2, "1.24"},
		{"9.999", 2, "10"},
		{"-9.999", 2, "-10"},
		{"0.999", 2, "1"},

		// Rounding all the way down to an integer.
		{"1.000000000000000001", 0, "1"},
		{"1.999999999999999999", 18, "1.999999999999999999"},
	}
	for _, tc := range tests {
		d := MustParse(tc.in)
		got, err := d.Round(tc.target)
		if err != nil {
			t.Errorf("%s.Round(%d) returned error %v, want %s", tc.in, tc.target, err, tc.out)
			continue
		}
		if s := got.String(); s != tc.out {
			t.Errorf("%s.Round(%d) = %s, want %s", tc.in, tc.target, s, tc.out)
		}
		requireCanonical(t, got, tc.in+".Round")
		if got.Scale() > int(tc.target) {
			t.Errorf("%s.Round(%d) has scale %d, want at most %d", tc.in, tc.target, got.Scale(), tc.target)
		}
	}
}

// TestRoundErrors covers the rejected targets, which mirror Rescale.
func TestRoundErrors(t *testing.T) {
	d := MustParse("1.5")
	for _, target := range []int8{-1, -18, 19, 100} {
		got, err := d.Round(target)
		if !errors.Is(err, ErrScaleOutOfRange) {
			t.Errorf("Round(%d) error = %v, want ErrScaleOutOfRange", target, err)
		}
		if got != (Decimal{}) {
			t.Errorf("Round(%d) = %v alongside its error, want the zero value", target, got)
		}
	}
	// Rounding up a huge coefficient still overflows rather than wrapping.
	if got, err := MustParse("9223372036854775807").Round(1); !errors.Is(err, ErrOverflow) {
		t.Errorf("MaxInt64.Round(1) = (%v, %v), want ErrOverflow", got, err)
	}
}

// TestRoundIsMonotonic and TestRoundHalfEvenBalance check two properties that
// a rounding implementation is easy to get subtly wrong.
func TestRoundIsMonotonic(t *testing.T) {
	previous := math.Inf(-1)
	for i := -1000; i <= 1000; i++ {
		d, err := FromCoefScale(int64(i), 1)
		if err != nil {
			t.Fatalf("FromCoefScale(%d, 1): %v", i, err)
		}
		rounded, err := d.Round(0)
		if err != nil {
			t.Fatalf("%s.Round(0): %v", d, err)
		}
		v := rounded.Float64()
		if v < previous {
			t.Fatalf("rounding %s to 0 gave %s, which is below the previous result %v", d, rounded, previous)
		}
		previous = v
	}
}

// TestRoundHalfEvenBalance checks that ties are split evenly between the
// upward and the downward neighbour, which is the point of half-to-even.
func TestRoundHalfEvenBalance(t *testing.T) {
	up, down := 0, 0
	for i := 0; i < 100; i++ {
		// The odd multiples of 0.5 are the ties: 0.5, 1.5, 2.5, ...
		d := MustParse(strconv.Itoa(i) + ".5")
		rounded, err := d.Round(0)
		if err != nil {
			t.Fatalf("%s.Round(0): %v", d, err)
		}
		iv, ok := rounded.Int64()
		if !ok {
			t.Fatalf("%s.Round(0) = %s is not an integer", d, rounded)
		}
		if iv%2 != 0 {
			t.Fatalf("%s.Round(0) = %d, want an even result", d, iv)
		}
		if int(iv) > i {
			up++
		} else {
			down++
		}
	}
	if up == 0 || down == 0 {
		t.Fatalf("half-to-even rounded %d up and %d down, want a balanced split", up, down)
	}
}

// TestRescaleWideningSucceeds covers the widening path that actually fits, which
// is the counterpart of the overflow cases above.
func TestRescaleWideningSucceeds(t *testing.T) {
	tests := []struct {
		in     string
		target int8
	}{
		{"1", 1},
		{"1", 18},
		{"1.5", 2},
		{"1.5", 18},
		{"-1.5", 3},
		{"0.001", 4},
		{"0.000000000000000001", 18},
		{"922337203685477580", 1},    // coef 922337203685477580 stretched by 10
		{"0.922337203685477580", 18}, // coef 922337203685477580 already at scale 18
		{"9223372036854775807e-18", 18},
	}
	for _, tc := range tests {
		d := MustParse(tc.in)
		if tc.target <= d.scale {
			continue
		}
		got, err := d.Rescale(tc.target)
		if err != nil {
			t.Errorf("%s.Rescale(%d) error = %v", tc.in, tc.target, err)
			continue
		}
		if !got.Equal(d) {
			t.Errorf("%s.Rescale(%d) = %s, which is a different value", tc.in, tc.target, got)
		}
		requireCanonical(t, got, tc.in+".Rescale")
	}
}

// TestX10Boundary covers the two arms of the overflow check: product exactly at
// the int64 bound, and one unit past it.
func TestX10Boundary(t *testing.T) {
	// 922337203685477580 * 10 is exactly MaxInt64 - 7, which fits.
	if got, ok := x10(922337203685477580, 1); !ok || got != 9223372036854775800 {
		t.Errorf("x10(922337203685477580, 1) = (%d, %v), want (9223372036854775800, true)", got, ok)
	}
	// 922337203685477581 * 10 exceeds MaxInt64.
	if _, ok := x10(922337203685477581, 1); ok {
		t.Error("x10(922337203685477581, 1) reported success, want overflow")
	}
	// A shift of zero is the identity, on both signs.
	if got, ok := x10(-5, 0); !ok || got != -5 {
		t.Errorf("x10(-5, 0) = (%d, %v), want (-5, true)", got, ok)
	}
	// The full range of shifts must be well defined.
	for p := int8(0); p <= MaxScale; p++ {
		if _, ok := x10(1, p); !ok {
			t.Errorf("x10(1, %d) reported overflow", p)
		}
	}
}
