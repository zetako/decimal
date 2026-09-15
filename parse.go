package decimal

// Parse converts a decimal literal into a Decimal.
//
// The accepted grammar is:
//
//	[+-]? digits ['.' digits?] [eE [+-]? digits]
//	[+-]? '.' digits  [eE [+-]? digits]
//
// That is, a sign is optional, at least one digit must be present either before
// or after the decimal point, and the exponent marker must be followed by at
// least one digit. Both "5." and ".5" are accepted, as are "1e3" and "1E+3".
// The exponent may be arbitrarily large or small; it is an error when the
// resulting value does not fit the representation.
//
// Rejected input includes the empty string, any surrounding or embedded
// whitespace, digit separators such as "1_000", the words "NaN", "Inf" and
// "Infinity", a second decimal point, and a dangling exponent such as "1e" or
// "1e+". A leading "+" is accepted; a leading "-" applies to the value, so
// "-0.0" parses to the canonical zero.
//
// Parse never rounds and never truncates: input with more than MaxScale
// fractional digits fails with ErrScaleOutOfRange, and a coefficient that does
// not fit in an int64 fails with ErrOverflow. Parse does not allocate on the
// heap on its success path.
func Parse(s string) (Decimal, error) {
	return parseString(s)
}

// MustParse is like Parse but panics if s is not a valid decimal literal. It is
// intended for tests and for package level constant initialisation, where a
// failure is a programming error rather than a runtime condition.
func MustParse(s string) Decimal {
	d, err := parseString(s)
	if err != nil {
		panic("decimal: MustParse(" + s + "): " + err.Error())
	}
	return d
}

// parseString parses s as a complete decimal literal. It is the single parser in
// the package: Parse, UnmarshalJSON, UnmarshalText and Scan all funnel here, so
// every input path shares the same grammar and the same bounds checks.
func parseString(s string) (Decimal, error) {
	// 1. Consume an optional sign.
	neg := false
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		neg = s[i] == '-'
		i++
	}

	var (
		mag     uint64 // absolute value of the accumulated digits
		digits  int    // number of accumulated digits
		fracLen int    // number of digits after the decimal point
		seenDot bool
		seenAny bool // at least one digit anywhere in mantissa or exponent
	)

	// 2. Consume the mantissa: digits, at most one '.', more digits.
	for i < len(s) {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			seenAny = true
			i++
			if seenDot {
				fracLen++
			}
			// Leading zeros carry no magnitude and must not count towards the
			// digit budget, otherwise "0.000000000000000000001" would look
			// like a 21 digit coefficient.
			if mag == 0 && c == '0' {
				continue
			}
			digits++
			if digits > 19 {
				// No 20 digit value fits in an int64 coefficient.
				return Decimal{}, errOverflow(s, "coefficient needs more than 19 digits")
			}
			mag = mag*10 + uint64(c-'0')
		case c == '.':
			if seenDot {
				return Decimal{}, errSyntax(s, "more than one decimal point")
			}
			seenDot = true
			i++
		default:
			// 3. Leave the loop at the exponent marker or at a junk byte.
			goto mantissaDone
		}
	}

mantissaDone:
	if digits == 0 && !seenAny {
		// The mantissa held no digit at all, so the input was empty, a lone
		// sign, a lone '.' or a leading exponent marker.
		return Decimal{}, errSyntax(s, "missing digits")
	}

	// 4. Consume an optional exponent.
	exp := 0
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		expNeg := false
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			expNeg = s[i] == '-'
			i++
		}
		if i >= len(s) || s[i] < '0' || s[i] > '9' {
			return Decimal{}, errSyntax(s, "exponent has no digits")
		}

		const expCap = 1 << 30 // far beyond any representable scale
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			if exp <= expCap {
				exp = exp*10 + int(s[i]-'0')
			}
			i++
		}
		if expNeg {
			exp = -exp
		}
	}
	if i != len(s) {
		return Decimal{}, errSyntax(s, "unexpected character "+quoteByte(s[i]))
	}

	// 5. Combine the mantissa and the exponent into a coefficient and a scale,
	// checking every bound explicitly.
	scale := int64(fracLen) - int64(exp)
	if scale > int64(MaxScale) {
		return Decimal{}, errScale(s, "more than 18 fractional digits")
	}
	if mag > maxCoefAbs {
		return Decimal{}, errOverflow(s, "coefficient does not fit in an int64")
	}

	// A negative scale means the exponent pushes the value above the digits that
	// were written out, so the coefficient has to grow by the matching power of
	// ten. "1e3" is the integer 1000, and an exponent beyond MaxScale cannot fit
	// in an int64 coefficient.
	coef := int64(mag)
	if scale < 0 {
		shift := -scale
		if shift > int64(MaxScale) {
			return Decimal{}, errOverflow(s, "value is too large: an integer coefficient of at most 19 digits is required")
		}
		grown, ok := x10(coef, int8(shift))
		if !ok {
			return Decimal{}, errOverflow(s, "value is too large: an integer coefficient of at most 19 digits is required")
		}
		coef = grown
		scale = 0
	}

	if neg {
		coef = -coef
	}
	return normalize(Decimal{coef: coef, scale: int8(scale)}), nil
}

// quoteByte renders one byte of rejected input for an error message.
func quoteByte(c byte) string {
	const hex = "0123456789abcdef"
	return "'" + string([]byte{c}) + "' (0x" + string([]byte{hex[c>>4], hex[c&0x0f]}) + ")"
}
