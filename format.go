package decimal

// maxStringLen bounds the length of a canonical representation: up to 19
// coefficient digits, a possible '.', up to 17 leading fractional zeros, and a
// sign.
const maxStringLen = 40

// String returns the canonical decimal representation of d: plain positional
// notation with a '-' sign for negative values, no thousands separators, no
// leading or trailing zeros in the fraction and no exponent notation. Integers
// are printed without a decimal point, and zero is always "0".
//
// String performs exactly one heap allocation, the result string itself, so it
// reports 1 alloc/op. The digits are produced in a stack-backed buffer first.
func (d Decimal) String() string {
	var buf [maxStringLen]byte
	start := encodeDecimal(&buf, d)
	// The slice expression keeps the array on the stack: only the string
	// conversion copies bytes to the heap.
	return string(buf[start:])
}

// encodeDecimal writes the canonical representation of d into buf and returns
// the index at which it starts, so that buf[start:] are the bytes of the
// representation. buf must have room for maxStringLen bytes.
//
// The representation is built left to right from a length computed up front,
// which keeps every index checkable: the fraction always holds exactly Scale()
// digits, and the significant digits end at the last byte of the window.
func encodeDecimal(buf *[maxStringLen]byte, d Decimal) int {
	// 1. Encode the coefficient digits into the tail of a scratch area, so
	// that sig holds exactly the significant digits. The digits of the value
	// always end at the last byte of the window.
	var digits [20]byte
	digitsLen := encodeUint64(&digits, coefAbs(d.coef))
	sig := digits[len(digits)-digitsLen:]

	// 2. Lay the digits out. A value needs a decimal point when its scale is
	// positive; the leading '0' that precedes the point in the sub-unit case is
	// then already covered by the window length.
	scale := int(d.scale)
	length := digitsLen
	switch {
	case scale == 0:
		// Integer: the digits as they are.
	case scale < digitsLen:
		// The point falls inside the digits.
		length++
	default:
		// The value is below one: '0', the point, zero padding, then the digits.
		length = scale + 2
	}
	if d.coef < 0 {
		length++
	}
	start := len(buf) - length

	// 3. Write the sign, then the body from left to right.
	i := start
	if d.coef < 0 {
		buf[i] = '-'
		i++
	}
	switch {
	case scale == 0:
		copy(buf[i:], sig)

	case scale < digitsLen:
		intLen := digitsLen - scale
		copy(buf[i:], sig[:intLen])
		buf[i+intLen] = '.'
		copy(buf[i+intLen+1:], sig[intLen:])

	default:
		// No integer part: a single '0', the point, zero padding up to the
		// coefficient's digits, which sit at the tail of the window.
		buf[i] = '0'
		buf[i+1] = '.'
		i += 2
		for end := len(buf) - digitsLen; i < end; i++ {
			buf[i] = '0'
		}
		copy(buf[i:], sig)
	}
	return start
}

// encodeUint64 writes the decimal digits of v into the tail of buf and returns
// the number of digits written, with encodeUint64(buf, 0) == 1. It is allocation
// free and exact for the whole uint64 range.
func encodeUint64(buf *[20]byte, v uint64) int {
	i := len(buf)
	for {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
		if v == 0 {
			break
		}
	}
	return len(buf) - i
}
