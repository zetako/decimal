package decimal

import "math/bits"

// Cmp compares d and other numerically and returns -1, 0 or +1. The comparison
// is exact and independent of the representation, so values with different
// scales compare correctly and zero compares equal to every canonical zero.
//
// d.Cmp(other) < 0 is equivalent to d.LessThan(other); Cmp never allocates.
func (d Decimal) Cmp(other Decimal) int {
	if d.coef == other.coef && d.scale == other.scale {
		return 0
	}
	a, b, _, ok := align(d, other)
	if !ok {
		// align only fails on an int64 wrap-around, which the comparison can
		// reproduce faithfully by comparing the 128 bit equivalents instead.
		return cmpWide(d, other)
	}
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// Equal reports whether d and other are numerically equal.
func (d Decimal) Equal(other Decimal) bool { return d.Cmp(other) == 0 }

// LessThan reports whether d is numerically less than other.
func (d Decimal) LessThan(other Decimal) bool { return d.Cmp(other) < 0 }

// GreaterThan reports whether d is numerically greater than other.
func (d Decimal) GreaterThan(other Decimal) bool { return d.Cmp(other) > 0 }

// align brings d and other to a common scale and returns their coefficients at
// that scale together with the scale itself. It reports false when stretching
// one of the operands would overflow an int64, for instance when comparing a
// huge integer against a value with a large scale.
//
// The common scale is the finer of the two, so exactly one operand is ever
// rescaled: the one whose scale is smaller is multiplied by 10^(difference).
// That multiplication is exact for canonical values and is the only step that
// can overflow.
func align(d, other Decimal) (int64, int64, int8, bool) {
	a, b := d.coef, other.coef
	switch {
	case d.scale == other.scale:
		return a, b, d.scale, true

	case d.scale > other.scale:
		// Bring the coarser operand up to the finer scale.
		var ok bool
		if b, ok = x10(b, d.scale-other.scale); !ok {
			return 0, 0, 0, false
		}
		return a, b, d.scale, true

	default:
		var ok bool
		if a, ok = x10(a, other.scale-d.scale); !ok {
			return 0, 0, 0, false
		}
		return a, b, other.scale, true
	}
}

// cmpWide compares d and other exactly by bringing both magnitudes to a common
// scale in a 128 bit intermediate. It is the fallback for operands whose
// alignment does not fit in an int64, and it never overflows itself: the largest
// quantity it forms is |coef| * 10^18, which is below 2^123.
//
// Both sides are multiplied up by a power of ten and never divided, so the
// comparison stays exact. The common scale is the finer of the two, because
// dividing to reach the coarser one would drop the fraction that distinguishes
// values such as 1e-18 from zero.
func cmpWide(d, other Decimal) int {
	ref := d.scale
	if other.scale > ref {
		ref = other.scale
	}
	aHi, aLo := mulPow10Wide(coefAbs(d.coef), ref-d.scale)
	bHi, bLo := mulPow10Wide(coefAbs(other.coef), ref-other.scale)
	mag := cmp128(aHi, aLo, bHi, bLo)

	// Compare magnitudes for equal signs, and negate the order when exactly one
	// operand is negative; two negatives reverse the magnitude order.
	switch {
	case d.coef < 0 && other.coef < 0:
		return -mag
	case d.coef < 0:
		return -1
	case other.coef < 0:
		return 1
	default:
		return mag
	}
}

// mulPow10Wide returns |coef| * 10^shift as a 128 bit unsigned value, split into
// high and low 64 bit halves. It takes a shift rather than a pair of scales so
// that the caller's intent cannot be misread: the parameter is the exponent of
// ten to apply, not the scale to interpret.
//
// shift is the difference between two scales, so it is in [0, MaxScale] for every
// value this package produces. Anything else would mean a scale escaped its
// bound, and clamping here would silently compare the wrong numbers, so it
// panics instead of guessing.
func mulPow10Wide(abs uint64, shift int8) (uint64, uint64) {
	if shift <= 0 {
		return 0, abs
	}
	if shift > MaxScale {
		panic("decimal: internal error: power-of-ten shift out of range")
	}
	return bits.Mul64(abs, uint64(pow10[shift]))
}

// cmp128 compares two 128 bit unsigned values given as high/low pairs.
func cmp128(aHi, aLo, bHi, bLo uint64) int {
	switch {
	case aHi != bHi:
		if aHi < bHi {
			return -1
		}
		return 1
	case aLo != bLo:
		if aLo < bLo {
			return -1
		}
		return 1
	default:
		return 0
	}
}
