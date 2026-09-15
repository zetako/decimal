package decimal

// Neg returns -d.
func (d Decimal) Neg() Decimal {
	if d.coef == 0 {
		return Decimal{}
	}
	return Decimal{coef: -d.coef, scale: d.scale}
}

// Abs returns |d|.
func (d Decimal) Abs() Decimal {
	if d.coef >= 0 {
		return d
	}
	return Decimal{coef: -d.coef, scale: d.scale}
}

// Add returns d + other exactly.
//
// The operands are first brought to a common scale, which is where the
// interesting overflow cases live: the aligned coefficients may not fit in an
// int64 even though both operands do, and the addition of two aligned
// coefficients may overflow as well. Both conditions are detected and reported
// as ErrOverflow, never silently wrapped. The result is canonical, so
// adding 0.5 and 0.5 gives 1, not 1.0.
func (d Decimal) Add(other Decimal) (Decimal, error) {
	a, b, scale, ok := align(d, other)
	if !ok {
		return Decimal{}, errOverflow("", "operands cannot be aligned to a common scale")
	}
	sum := a + b
	// Go defines signed overflow as wrapping, so the range is checked by hand.
	// Overflow happens when both operands share a sign and push the result past
	// the bound, which shows up as the sum disagreeing with that sign; the
	// magnitudes of a and b are at most MaxInt64, so no other wrap is possible.
	if (a > 0 && b > 0 && sum < 0) || (a < 0 && b < 0 && sum > 0) {
		return Decimal{}, errOverflow("", "sum of aligned coefficients does not fit in an int64")
	}
	// MinInt64 is exactly representable, but it is not a usable coefficient:
	// the magnitude of a value has to stay negatable, so |MinInt64| must fit as
	// well, and it does not.
	if sum == minInt64 {
		return Decimal{}, errOverflow("", "sum of aligned coefficients cannot be MinInt64")
	}
	return normalize(Decimal{coef: sum, scale: scale}), nil
}

// minInt64 is the smallest int64 value. It is never a valid coefficient, since
// its magnitude |MinInt64| is one larger than MaxInt64 and so could not be
// negated or scaled back.
const minInt64 = -1 << 63

// Sub returns d - other exactly. It reports ErrOverflow under the same
// conditions as Add.
func (d Decimal) Sub(other Decimal) (Decimal, error) {
	return d.Add(other.Neg())
}

// MulInt returns d * i exactly. It reports ErrOverflow when the product does
// not fit in an int64 coefficient, and never rounds.
func (d Decimal) MulInt(i int64) (Decimal, error) {
	// Multiplication by 0 and 1 cannot overflow, so they skip the check.
	if i == 0 {
		return Decimal{}, nil
	}
	if i == 1 {
		return d, nil
	}
	prod, ok := checkedMul(d.coef, i)
	if !ok {
		return Decimal{}, errOverflow("", "product does not fit in an int64 coefficient")
	}
	return normalize(Decimal{coef: prod, scale: d.scale}), nil
}

// Rescale returns d represented with exactly the given scale, without changing
// its value.
//
// Increasing the scale multiplies the coefficient by a power of ten and is
// allowed only when the result still fits in an int64. Decreasing the scale
// divides the coefficient and is allowed only when the division is exact, so
// no precision is ever lost: Rescale(4) on 1.25 fails, while Rescale(1) on 1.50
// succeeds because 1.50 is stored as the canonical 1.5 and 1.5 cannot be
// represented at scale 1 any other way. Use Round when a lossy conversion is
// intended.
//
// The returned value is canonical, so its Scale() may be smaller than the
// requested scale when the value ends in zeros (rescaling 12.5 to scale 3 gives
// the canonical 12.5 at scale 1, not 12.500).
//
// It returns an error wrapping ErrScaleOutOfRange when target is outside
// [0, MaxScale], and one wrapping ErrOverflow when scaling up is not
// representable.
func (d Decimal) Rescale(target int8) (Decimal, error) {
	if target < 0 || target > MaxScale {
		return Decimal{}, errScale("", "target scale must be in [0, 18]")
	}
	if target == d.scale {
		return d, nil
	}
	if target > d.scale {
		coef, ok := x10(d.coef, target-d.scale)
		if !ok {
			return Decimal{}, errOverflow("", "rescaling would not fit in an int64 coefficient")
		}
		return normalize(Decimal{coef: coef, scale: target}), nil
	}
	coef := d.coef / pow10[d.scale-target]
	if coef*pow10[d.scale-target] != d.coef {
		return Decimal{}, errOverflow("", "rescaling would lose precision")
	}
	return normalize(Decimal{coef: coef, scale: target}), nil
}

// Round returns d rounded to exactly the given scale using half-to-even
// rounding, the mode that avoids the upward bias of "round half away from
// zero". It is the only rounding operation in this package; every other
// operation either computes an exact result or fails.
//
// Rounding to a scale greater than or equal to Scale() is exact and behaves
// like Rescale, except that it may still fail with ErrOverflow. It returns an
// error wrapping ErrScaleOutOfRange when target is outside [0, MaxScale].
//
//	d, _ := decimal.Parse("2.5")
//	r, _ := d.Round(0)      // 2, because 2 is even
//	d, _ = decimal.Parse("3.5")
//	r, _ = d.Round(0)       // 4
//	d, _ = decimal.Parse("-2.5")
//	r, _ = d.Round(0)       // -2
func (d Decimal) Round(target int8) (Decimal, error) {
	if target < 0 || target > MaxScale {
		return Decimal{}, errScale("", "target scale must be in [0, 18]")
	}
	if target >= d.scale {
		return d.Rescale(target)
	}

	// 1. Split d into the target-scale quotient and the dropped remainder.
	drop := d.scale - target
	factor := pow10[drop]
	quot := d.coef / factor
	rem := d.coef % factor

	// 2. Decide whether the quotient moves one step away from zero. Go's
	// division truncates towards zero, so the remainder carries the sign of the
	// coefficient and the step has to follow it: rounding a negative value up
	// to the next multiple of ten would move it towards zero, which is the
	// wrong direction.
	absRem := coefAbs(rem)
	half := uint64(factor) / 2
	away := 0
	switch {
	case absRem > half:
		away = 1
	case absRem < half:
		// Below the midpoint: truncation is already the nearest value.
	case quot%2 != 0:
		// Exactly at the midpoint with an odd quotient: round to even.
		away = 1
	default:
		// Exactly at the midpoint with an even quotient: keep it.
	}
	if d.coef < 0 {
		quot -= int64(away)
	} else {
		quot += int64(away)
	}
	return normalize(Decimal{coef: quot, scale: target}), nil
}

// checkedMul multiplies two int64 values and reports whether the product fits
// in an int64. Go defines signed overflow as wrapping, so the bound has to be
// checked by hand before multiplying.
func checkedMul(a, b int64) (int64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	prod := a * b
	// Undo the multiplication; a wrapped product does not divide back exactly.
	if prod/b != a {
		return 0, false
	}
	return prod, true
}

// x10 multiplies coef by 10^p and reports whether the result fits in an int64.
// p must be in [0, MaxScale].
func x10(coef int64, p int8) (int64, bool) {
	if p == 0 {
		return coef, true
	}
	factor := pow10[p]
	abs := coefAbs(coef)
	limit := maxCoefAbs / uint64(factor)
	switch {
	case abs > limit:
		return 0, false
	case abs < limit:
		return coef * factor, true
	case abs%uint64(factor) == 0:
		// Exactly at the bound: the product is representable.
		return coef * factor, true
	default:
		return 0, false
	}
}
