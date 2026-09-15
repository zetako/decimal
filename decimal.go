package decimal

// MaxScale is the largest supported number of decimal places. It follows from
// the int64 coefficient: 10^-18 is the smallest non-zero magnitude that is
// still representable, since a coefficient of 1 shifted by 19 places would no
// longer be an int64 digit pattern that survives normalization.
const MaxScale int8 = 18

// pow10 holds the exact powers of ten up to 10^MaxScale. Every entry fits in an
// int64, which is the property that bounds the magnitudes accepted here.
var pow10 = [...]int64{
	1, 1e1, 1e2, 1e3, 1e4, 1e5, 1e6, 1e7, 1e8,
	1e9, 1e10, 1e11, 1e12, 1e13, 1e14, 1e15, 1e16, 1e17, 1e18,
}

// Decimal is an exact bounded fixed-point decimal: the value it holds is
// coef * 10^-scale, with coef in the int64 range and scale in [0, MaxScale].
//
// The zero value of Decimal is the number zero, and every value reachable
// through this package is canonical:
//
//   - the sign is carried by the coefficient (there is no sign field);
//   - zero is coef == 0 && scale == 0, so "-0.0" becomes plain 0;
//   - there are no trailing zeros in the fraction, and scale is 0 if and only
//     if the value is an integer.
//
// Decimal is immutable and passed by value: no method modifies its receiver and
// every operation returns a new Decimal. Use Cmp, Equal, LessThan or
// GreaterThan to compare numbers; == compares the canonical representation, not
// the numeric value.
type Decimal struct {
	coef  int64 // coefficient, carries the sign
	scale int8  // number of decimal places, always in [0, MaxScale]
}

// Zero returns the canonical zero value.
func Zero() Decimal { return Decimal{} }

// FromInt returns the Decimal that represents i exactly.
func FromInt(i int64) Decimal { return Decimal{coef: i} }

// FromCoefScale returns the Decimal coef * 10^-scale in canonical form.
//
// It returns an error wrapping ErrScaleOutOfRange when scale is outside
// [0, MaxScale]. Trailing zeros are removed and a zero coefficient is collapsed
// to the canonical zero, so FromCoefScale(150, 2) and FromCoefScale(15, 1) are
// both 1.5, and every FromCoefScale(0, s) is 0.
func FromCoefScale(coef int64, scale int8) (Decimal, error) {
	if scale < 0 || scale > MaxScale {
		return Decimal{}, errScale("", "scale must be in [0, 18]")
	}
	return normalize(Decimal{coef: coef, scale: scale}), nil
}

// normalize returns d in canonical form: zero collapsed to scale 0, and no
// trailing zeros left in the fraction. It never changes the numeric value.
func normalize(d Decimal) Decimal {
	if d.coef == 0 {
		return Decimal{}
	}

	// 1. Strip trailing zeros of the fraction while the scale is positive.
	for d.scale > 0 && d.coef%10 == 0 {
		d.coef /= 10
		d.scale--
	}
	return d
}

// Scale returns the number of decimal places, always in [0, MaxScale].
func (d Decimal) Scale() int { return int(d.scale) }

// Coef returns the int64 coefficient; the represented value is
// Coef() * 10^-Scale().
func (d Decimal) Coef() int64 { return d.coef }

// IsZero reports whether d is numerically zero. Any Decimal from this package
// canonicalises zero to the value Zero(), so this is equivalent to d == Zero().
func (d Decimal) IsZero() bool { return d.coef == 0 }

// IsInt reports whether d has no fractional part. Because values are canonical,
// that is exactly Scale() == 0.
func (d Decimal) IsInt() bool { return d.scale == 0 }

// Sign returns -1 if d < 0, 0 if d == 0 and +1 if d > 0.
func (d Decimal) Sign() int {
	switch {
	case d.coef < 0:
		return -1
	case d.coef > 0:
		return 1
	default:
		return 0
	}
}

// Int64 returns the value of d as an int64. It reports false when d has a
// fractional part or when its magnitude exceeds the int64 range; it never
// truncates and never rounds.
func (d Decimal) Int64() (int64, bool) {
	if d.scale != 0 {
		return 0, false
	}
	return d.coef, true
}

// Float64 returns the value of d converted to a float64.
//
// This conversion is provided for display and interoperability only. It is
// inexact in two ways: whenever the coefficient needs more than 53 bits of
// precision, and whenever the value's magnitude is large enough that the
// fractional digits fall below the resolution of the format, as in
// 9223372036854775807e-18, where most of the fraction disappears. Small values
// near the representable floor, such as 1e-18 itself, do convert exactly.
//
// Use Cmp or Equal, never Float64, to decide anything about a value.
func (d Decimal) Float64() float64 {
	f := float64(d.coef)
	if d.scale == 0 {
		return f
	}
	return f / float64(pow10[d.scale])
}

// maxCoefAbs is the largest usable magnitude of a coefficient, |math.MinInt64|
// being excluded because its negation is not representable.
const maxCoefAbs = uint64(1)<<63 - 1

// coefAbs returns |coef| as a uint64. The int64 minimum is handled without
// negating it, so the operation cannot overflow.
func coefAbs(coef int64) uint64 {
	if coef < 0 {
		return uint64(-(coef + 1)) + 1
	}
	return uint64(coef)
}
