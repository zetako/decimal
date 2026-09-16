package decimal

import "testing"

// isCanonical reports whether d satisfies every documented invariant:
// a scale inside [0, MaxScale], a coefficient other than MinInt64, no trailing
// zeros in the fraction, the single zero representation, and a value that agrees
// with the coefficient.
//
// It is a test-only helper, deliberately not part of the exported API: callers
// never need to ask, because every Decimal this package hands out is canonical
// by construction.
func isCanonical(d Decimal) bool {
	if d.scale < 0 || d.scale > MaxScale {
		return false
	}
	if d.coef == minInt64 {
		return false
	}
	if d.coef == 0 {
		return d.scale == 0
	}
	if d.scale > 0 && d.coef%10 == 0 {
		return false
	}
	return true
}

// requireCanonical fails the test when d breaks an invariant, and reports which
// one.
func requireCanonical(t *testing.T, d Decimal, ctx string) {
	t.Helper()
	if isCanonical(d) {
		return
	}
	switch {
	case d.scale < 0 || d.scale > MaxScale:
		t.Fatalf("%s: scale %d is outside [0, %d]", ctx, d.scale, MaxScale)
	case d.coef == minInt64:
		t.Fatalf("%s: coefficient is MinInt64, whose magnitude is not negatable", ctx)
	case d.coef == 0 && d.scale != 0:
		t.Fatalf("%s: zero is not canonical: coef=0 scale=%d", ctx, d.scale)
	case d.coef%10 == 0:
		t.Fatalf("%s: value %v has a trailing zero in the fraction", ctx, d)
	default:
		t.Fatalf("%s: %v is not canonical", ctx, d)
	}
}

// mustFromInt is FromInt with the error turned into a panic, for the test corpus:
// every integer used there is representable, so a failure is a bug in the test
// rather than a case under test.
func mustFromInt(i int64) Decimal {
	d, err := FromInt(i)
	if err != nil {
		panic(err)
	}
	return d
}
