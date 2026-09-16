package decimal

import (
	"math"
	"sort"
	"testing"
)

// TestCmp covers the total order: negatives, values across different scales,
// the canonical zero and the extremes of the range.
func TestCmp(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		// Equal values, including equal values written differently.
		{"0", "0", 0},
		{"0", "-0.0", 0},
		{"1", "1", 0},
		{"1.5", "1.50", 0},
		{"1", "1.000", 0},
		{"1e3", "1000", 0},
		{"0.1", "0.10", 0},
		{"-1.5", "-1.50", 0},
		{"1e-18", "0.000000000000000001", 0},
		{"9223372036854775807", "9223372036854775807", 0},

		// Ordering within one sign.
		{"1", "2", -1},
		{"2", "1", 1},
		{"-1", "-2", 1},
		{"-2", "-1", -1},
		{"0", "1", -1},
		{"0", "-1", 1},
		{"-1", "0", -1},
		{"1", "0", 1},

		// Ordering across scales, where a naive comparison of coefficients
		// would be wrong.
		{"0.5", "1", -1},
		{"1.5", "1", 1},
		{"0.000000000000000001", "0", 1},
		{"0.000000000000000001", "0.000000000000000002", -1},
		{"1.000000000000000001", "1", 1},
		{"9223372036854775807", "0.000000000000000001", 1},
		{"-9223372036854775807", "0.000000000000000001", -1},
		{"-0.000000000000000001", "0", -1},
		{"0.1", "0.099999999999999999", 1},
		{"1000000", "999999.999999999999", 1},

		// The alignment of these two overflows an int64, so Cmp falls back to
		// its 128 bit path: MaxInt64 at scale 0 against 1e-18.
		{"9223372036854775807", "-0.000000000000000001", 1},
		{"-9223372036854775807", "-0.000000000000000001", -1},

		// Same magnitude, different signs.
		{"1", "-1", 1},
		{"-1", "1", -1},
		{"1.5", "-1.5", 1},
	}

	for _, tc := range tests {
		t.Run(tc.a+"_"+tc.b, func(t *testing.T) {
			a, b := MustParse(tc.a), MustParse(tc.b)
			if got := a.Cmp(b); got != tc.want {
				t.Errorf("%s.Cmp(%s) = %d, want %d", tc.a, tc.b, got, tc.want)
			}
			// The comparison must be antisymmetric and agree with the
			// convenience predicates.
			if got := b.Cmp(a); got != -tc.want {
				t.Errorf("%s.Cmp(%s) = %d, want %d (antisymmetry)", tc.b, tc.a, got, -tc.want)
			}
			if got := a.Equal(b); got != (tc.want == 0) {
				t.Errorf("%s.Equal(%s) = %v, want %v", tc.a, tc.b, got, tc.want == 0)
			}
			if got := a.LessThan(b); got != (tc.want < 0) {
				t.Errorf("%s.LessThan(%s) = %v, want %v", tc.a, tc.b, got, tc.want < 0)
			}
			if got := a.GreaterThan(b); got != (tc.want > 0) {
				t.Errorf("%s.GreaterThan(%s) = %v, want %v", tc.a, tc.b, got, tc.want > 0)
			}
		})
	}
}

// TestCmpZeroIsZero pins that every spelling of zero compares equal, which is
// the observable consequence of the single zero representation.
func TestCmpZeroIsZero(t *testing.T) {
	zeros := []string{"0", "-0", "+0", "0.0", "-0.000", "0e10", "-0e-10", "0.000000000000000000"}
	for _, a := range zeros {
		for _, b := range zeros {
			da, db := MustParse(a), MustParse(b)
			if da.Cmp(db) != 0 || !da.Equal(db) {
				t.Fatalf("%s and %s must be the same number (got %d)", a, b, da.Cmp(db))
			}
			if da != db {
				t.Fatalf("%s and %s must also have the same representation", a, b)
			}
			if da.Sign() != 0 {
				t.Fatalf("%s.Sign() = %d, want 0", a, da.Sign())
			}
		}
	}
}

// TestCmpIsATotalOrder checks the ordering axioms on a set of values that mixes
// scales, signs and the extremes of the range.
func TestCmpIsATotalOrder(t *testing.T) {
	// Ascending numeric order: the closer a negative value is to zero, the
	// larger it is, so -1e-18 comes after -0.5 and before 0.
	literals := []string{
		"-9223372036854775807", "-1000", "-1.5", "-0.5", "-0.000000000000000001",
		"0", "0.000000000000000001", "0.5", "1", "1.5", "1000", "9223372036854775807",
	}
	vals := make([]Decimal, len(literals))
	for i, s := range literals {
		vals[i] = MustParse(s)
	}

	// 1. Every pair must be ordered consistently, and must not be ordered both
	// ways.
	for i := range vals {
		for j := range vals {
			ij, ji := vals[i].Cmp(vals[j]), vals[j].Cmp(vals[i])
			if ij != -ji {
				t.Fatalf("Cmp(%v, %v) = %d but Cmp(%v, %v) = %d", vals[i], vals[j], ij, vals[j], vals[i], ji)
			}
			if ij == 0 && !vals[i].Equal(vals[j]) {
				t.Fatalf("%v.Cmp(%v) == 0 but Equal is false", vals[i], vals[j])
			}
		}
	}

	// 2. Transitivity.
	for i := range vals {
		for j := range vals {
			for k := range vals {
				if vals[i].Cmp(vals[j]) < 0 && vals[j].Cmp(vals[k]) < 0 && vals[i].Cmp(vals[k]) >= 0 {
					t.Fatalf("order is not transitive for %v < %v < %v", vals[i], vals[j], vals[k])
				}
			}
		}
	}

	// 3. Sorting must reproduce the literal order, which is ascending and
	// unique.
	shuffled := make([]Decimal, len(vals))
	copy(shuffled, vals)
	for i := len(shuffled) - 1; i > 0; i-- {
		j := i / 2 // deterministic interleave instead of a random shuffle
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	}
	sort.Slice(shuffled, func(a, b int) bool { return shuffled[a].LessThan(shuffled[b]) })
	for i := range shuffled {
		if shuffled[i] != vals[i] {
			t.Fatalf("sorted[%d] = %v, want %v", i, shuffled[i], vals[i])
		}
	}
}

// TestCmpWidePath exercises the 128 bit fallback directly, including the case
// where both operands are negative.
func TestCmpWidePath(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"9223372036854775807", "0.000000000000000001", 1},
		{"0.000000000000000001", "9223372036854775807", -1},
		{"-9223372036854775807", "-0.000000000000000001", -1},
		{"-0.000000000000000001", "-9223372036854775807", 1},
		{"-9223372036854775807", "0.000000000000000001", -1},
		{"0.000000000000000001", "-9223372036854775807", 1},
	}
	for _, tc := range tests {
		a, b := MustParse(tc.a), MustParse(tc.b)
		// Confirm the fast path really cannot serve this pair, then check that
		// the fallback agrees anyway.
		if _, _, _, ok := align(a, b); ok {
			t.Fatalf("%s vs %s unexpectedly aligned in 64 bits", tc.a, tc.b)
		}
		if got := a.Cmp(b); got != tc.want {
			t.Errorf("%s.Cmp(%s) = %d, want %d (wide path)", tc.a, tc.b, got, tc.want)
		}
	}
}

// TestAlignNeverLies checks that whenever align succeeds it returns exactly the
// coefficients of both operands at the common scale.
func TestAlignNeverLies(t *testing.T) {
	pairs := [][2]string{
		{"1", "1"}, {"1.5", "1"}, {"1", "1.5"}, {"0", "0.001"},
		{"9223372036854775807", "0.000000000000000001"},
		{"-1.25", "3"}, {"1e-18", "1e-18"}, {"100", "0.01"},
	}
	for _, p := range pairs {
		a, b := MustParse(p[0]), MustParse(p[1])
		ca, cb, scale, ok := align(a, b)
		if !ok {
			continue
		}
		if got, err := FromCoefScale(ca, scale); err != nil || !got.Equal(a) {
			t.Fatalf("align(%s, %s) coefficient %d at scale %d is not %s (got %v, err %v)",
				p[0], p[1], ca, scale, p[0], got, err)
		}
		if got, err := FromCoefScale(cb, scale); err != nil || !got.Equal(b) {
			t.Fatalf("align(%s, %s) coefficient %d at scale %d is not %s (got %v, err %v)",
				p[0], p[1], cb, scale, p[1], got, err)
		}
	}
}

// TestAlignBoundaries pins the exact condition under which alignment gives up:
// the coarser operand cannot be stretched to the finer scale. It also checks
// that giving up never changes the answer, because Cmp falls back to 128 bit
// reasoning for precisely these pairs.
func TestAlignBoundaries(t *testing.T) {
	big := mustFromInt(math.MaxInt64)         // 9223372036854775807
	unit := MustParse("0.000000000000000001") // 1e-18

	// Stretching MaxInt64 by 10^18 does not fit in an int64.
	if _, _, _, ok := align(big, unit); ok {
		t.Fatal("aligning MaxInt64 with 1e-18 must fail")
	}
	// Alignment is symmetric in the operand order.
	if _, _, _, ok := align(unit, big); ok {
		t.Fatal("aligning 1e-18 with MaxInt64 must fail")
	}
	// A single digit still fits at scale 18: 9 * 10^18 overflows, so even that
	// must fail, while 0.9 * 10^18 does not.
	if _, _, _, ok := align(mustFromInt(9), unit); ok {
		t.Fatal("aligning 9 with 1e-18 must fail, since 9e18 exceeds MaxInt64")
	}
	if _, _, _, ok := align(MustParse("0.9"), unit); !ok {
		t.Fatal("aligning 0.9 with 1e-18 must succeed")
	}
	// Even a factor of ten is one too many for MaxInt64.
	if _, _, _, ok := align(big, MustParse("0.1")); ok {
		t.Fatal("aligning MaxInt64 with 0.1 must fail, since 10*MaxInt64 overflows")
	}
	// One notch down the range, the same alignment works.
	if _, _, _, ok := align(mustFromInt(math.MaxInt64/10), MustParse("0.1")); !ok {
		t.Fatal("aligning MaxInt64/10 with 0.1 must succeed")
	}

	// The fallback must still order the unalignable pairs correctly.
	if big.Cmp(unit) <= 0 {
		t.Fatalf("MaxInt64 must be greater than 1e-18, got %d", big.Cmp(unit))
	}
	if unit.Cmp(big) >= 0 {
		t.Fatalf("1e-18 must be smaller than MaxInt64, got %d", unit.Cmp(big))
	}
	if big.Cmp(MustParse("0.1")) <= 0 {
		t.Fatalf("MaxInt64 must be greater than 0.1, got %d", big.Cmp(MustParse("0.1")))
	}
}

// TestCmpWideShiftIsExact covers the 128 bit path for operands whose scale
// difference is the maximum, which is the largest multiplication the fallback
// ever performs.
func TestCmpWideShiftIsExact(t *testing.T) {
	// 9 at scale 0 against 1 at scale 18: the scale difference is MaxScale, so
	// the fallback multiplies by the largest power of ten it can, and 9 * 10^18
	// is past the int64 bound, which is what forces the wide path. Note that 1
	// against 1e-18 would still fit exactly and stay on the fast path.
	nine := mustFromInt(9)
	small := MustParse("0.000000000000000001")
	if _, _, _, ok := align(nine, small); ok {
		t.Fatal("stretching 9 to scale 18 must overflow an int64, forcing the wide path")
	}
	if got := nine.Cmp(small); got != 1 {
		t.Fatalf("9.Cmp(1e-18) = %d, want 1", got)
	}
	if got := small.Cmp(nine); got != -1 {
		t.Fatalf("1e-18.Cmp(9) = %d, want -1", got)
	}
	// And the same pair negated, where the magnitude order has to invert.
	if got := nine.Neg().Cmp(small.Neg()); got != -1 {
		t.Fatalf("-9.Cmp(-1e-18) = %d, want -1", got)
	}
	// The boundary case stays on the fast path, and the two paths agree.
	one := mustFromInt(1)
	if _, _, _, ok := align(one, small); !ok {
		t.Fatal("stretching 1 to scale 18 fits exactly and must not fall back")
	}
	if got, want := one.Cmp(small), cmpWide(one, small); got != want {
		t.Fatalf("the fast path says %d but the wide path says %d for 1 vs 1e-18", got, want)
	}
}

// TestMulPow10WideRejectsBadShift pins that an impossible shift is a loud
// internal error rather than a silently wrong comparison.
func TestMulPow10WideRejectsBadShift(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("a shift beyond MaxScale must panic")
		}
	}()
	mulPow10Wide(1, MaxScale+1)
}
