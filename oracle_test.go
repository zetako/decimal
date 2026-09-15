package decimal

import (
	"errors"
	"math"
	"math/big"
	"math/rand"
	"strconv"
	"strings"
	"testing"
)

// TestOracleParse compares Parse against math/big.Rat on a fixed corpus of
// literals that the representation must accept.
//
// big.Rat is used as a test-only oracle: it is arbitrary precision, so it is
// exact for every literal considered here, and it never touches the code under
// test. Production code in this package must not import math/big.
func TestOracleParse(t *testing.T) {
	corpus := []string{
		"0", "1", "-1", "1.5", "-1.5", "0.5", "1e-18", "-1e-18",
		"9223372036854775807", "-9223372036854775807",
		"9223372036854775807e-18", "9223372036854775807e-1",
		"0.000000000000000001", "1.000000000000000001",
		"1234567890123456789", "0.123456789012345678",
		"1e18", "1e17", "1e-17", "1e-1", "100", "0.001",
		"9.223372036854775807", "0.000000000000000000001e21",
	}
	for _, s := range corpus {
		d, err := Parse(s)
		if err != nil {
			t.Fatalf("Parse(%q) error = %v", s, err)
		}
		want, ok := new(big.Rat).SetString(s)
		if !ok {
			t.Fatalf("the oracle cannot parse %q", s)
		}
		if got := d.rat(); got.Cmp(want) != 0 {
			t.Fatalf("Parse(%q) = %s, but big.Rat says %s", s, got.RatString(), want.RatString())
		}
		requireCanonical(t, d, "Parse("+s+")")
	}
}

// TestOracleRandomLiterals is the main differential test. It generates random
// literals whose value fits the representation, then checks that Parse, Cmp, Add
// and Sub agree with big.Rat exactly.
func TestOracleRandomLiterals(t *testing.T) {
	rng := rand.New(rand.NewSource(20240915))

	for i := 0; i < 20000; i++ {
		a := randomFittingLiteral(rng)
		b := randomFittingLiteral(rng)

		da, ra, err := parseWithOracle(t, a)
		if err != nil {
			t.Fatalf("literal %q generated as fitting was rejected: %v", a, err)
		}
		db, rb, err := parseWithOracle(t, b)
		if err != nil {
			t.Fatalf("literal %q generated as fitting was rejected: %v", b, err)
		}

		// 1. Comparison must follow the numeric order exactly.
		wantCmp := ra.Cmp(rb)
		if got := da.Cmp(db); got != wantCmp {
			t.Fatalf("Cmp(%q, %q) = %d, want %d (%s vs %s)",
				a, b, got, wantCmp, ra.RatString(), rb.RatString())
		}

		// 2. Addition agrees whenever it is representable, and reports
		// overflow exactly when the exact sum leaves the representation.
		exactSum := new(big.Rat).Add(ra, rb)
		gotSum, err := da.Add(db)
		checkAgainstOracle(t, "Add", a, b, exactSum, gotSum, err)

		// 3. Subtraction, likewise.
		exactDiff := new(big.Rat).Sub(ra, rb)
		gotDiff, err := da.Sub(db)
		checkAgainstOracle(t, "Sub", a, b, exactDiff, gotDiff, err)
	}
}

// TestOracleRandomExtremes runs the same differential checks on literals that
// sit at the edge of the range, where overflow is the expected outcome.
func TestOracleRandomExtremes(t *testing.T) {
	rng := rand.New(rand.NewSource(4242))
	// Every coefficient has at most 18 digits, so Parse accepts all of them at
	// every exponent below; the arithmetic between them is where overflow lives.
	edgeCoefs := []string{
		"922337203685477580", "-922337203685477580", "9223372036854775806",
		"461168601842738790", "-461168601842738790", "1", "-1",
		"999999999999999999", "-999999999999999999", "100000000000000000",
	}
	scales := []string{"", "-18", "-1", "0", "1", "18"}

	for i := 0; i < 5000; i++ {
		a := pick(rng, edgeCoefs) + pick(rng, scales)
		b := pick(rng, edgeCoefs) + pick(rng, scales)

		da, ok := parseFitting(a)
		db, ok2 := parseFitting(b)
		if !ok || !ok2 {
			continue
		}
		ra, _ := new(big.Rat).SetString(a)
		rb, _ := new(big.Rat).SetString(b)

		if got, want := da.Cmp(db), ra.Cmp(rb); got != want {
			t.Fatalf("Cmp(%q, %q) = %d, want %d", a, b, got, want)
		}

		gotSum, err := da.Add(db)
		checkAgainstOracle(t, "Add", a, b, new(big.Rat).Add(ra, rb), gotSum, err)
		gotDiff, err := da.Sub(db)
		checkAgainstOracle(t, "Sub", a, b, new(big.Rat).Sub(ra, rb), gotDiff, err)
	}
}

// TestOracleLiteralSweep walks every combination in a dense grid of small
// literals, so the differential check also covers the systematic cases rather
// than only the random ones.
func TestOracleLiteralSweep(t *testing.T) {
	coefs := []string{"-1000000000000000000", "-150", "-100", "-1", "0", "1", "100", "150", "997", "1000000000000000000"}
	scales := []int{0, 1, 2, 3, 6, 17, 18}

	for _, c := range coefs {
		base, err := strconv.ParseInt(c, 10, 64)
		if err != nil {
			t.Fatalf("bad coefficient literal %q: %v", c, err)
		}
		for _, scale := range scales {
			d, err := FromCoefScale(base, int8(scale))
			if err != nil {
				t.Fatalf("FromCoefScale(%d, %d): %v", base, scale, err)
			}
			want := new(big.Rat).SetFrac(big.NewInt(base), pow10Rat(scale))
			if got := d.rat(); got.Cmp(want) != 0 {
				t.Fatalf("FromCoefScale(%d, %d) = %s, want %s", base, scale, got.RatString(), want.RatString())
			}
			// The text form must parse back to the same value.
			back, err := Parse(d.String())
			if err != nil {
				t.Fatalf("Parse(%q) from %d/%d: %v", d.String(), base, scale, err)
			}
			if back != d {
				t.Fatalf("Parse(%q) = %v, want %v", d.String(), back, d)
			}
			// And the float64 view must be within a few ULP of the oracle.
			oracle, _ := want.Float64()
			if diff := math.Abs(d.Float64() - oracle); diff > math.Abs(oracle)*1e-15+1e-300 {
				t.Fatalf("Float64(%s) = %g, oracle %g", d, d.Float64(), oracle)
			}
		}
	}
}

// TestOracleFloat64 checks the documented inexactness of Float64 against the
// oracle, in the direction that matters: the value is close, and the conversion
// is monotonic.
func TestOracleFloat64(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	for i := 0; i < 5000; i++ {
		s := randomFittingLiteral(rng)
		d, ok := parseFitting(s)
		if !ok {
			continue
		}
		want, ok := new(big.Rat).SetString(s)
		if !ok {
			continue
		}
		oracle, _ := want.Float64()
		got := d.Float64()
		// Allow a relative error of a few ULP, which is what double rounding
		// through a 53 bit coefficient costs.
		if diff := math.Abs(got - oracle); diff > math.Abs(oracle)*1e-15+1e-300 {
			t.Fatalf("Float64(%q) = %g, oracle float of %s = %g", s, got, want.RatString(), oracle)
		}
	}
}

// TestOracleStringRoundTrip checks that String is the exact inverse of Parse for
// the whole corpus, which is the property the JSON and SQL paths rely on.
func TestOracleStringRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 20000; i++ {
		s := randomFittingLiteral(rng)
		d, ok := parseFitting(s)
		if !ok {
			continue
		}
		back, err := Parse(d.String())
		if err != nil {
			t.Fatalf("Parse(%q) from %q: %v", d.String(), s, err)
		}
		if back != d {
			t.Fatalf("String produced %q for %q, which parses back to a different value", d.String(), s)
		}
		// Idempotence: formatting twice cannot drift.
		if again := back.String(); again != d.String() {
			t.Fatalf("String is not idempotent: %q then %q", d.String(), again)
		}
	}
}

// TestOracleOverflowIsReported checks the negative direction of every operation:
// when the exact result does not fit, the operation must say so instead of
// returning a wrapped value.
func TestOracleOverflowIsReported(t *testing.T) {
	rng := rand.New(rand.NewSource(31337))
	reported := 0

	for i := 0; i < 50000; i++ {
		a := randomAnyLiteral(rng)
		b := randomAnyLiteral(rng)

		da, oka := parseFitting(a)
		db, okb := parseFitting(b)
		if !oka || !okb {
			// This generator deliberately produces out of range literals, and
			// only the two range sentinels may come back.
			for _, lit := range []string{a, b} {
				if _, err := Parse(lit); err != nil &&
					!errors.Is(err, ErrOverflow) && !errors.Is(err, ErrScaleOutOfRange) {
					t.Fatalf("Parse(%q) error = %v, want a range error", lit, err)
				}
			}
			continue
		}

		ra, ok1 := new(big.Rat).SetString(a)
		rb, ok2 := new(big.Rat).SetString(b)
		if !ok1 || !ok2 {
			continue
		}

		sum, err := da.Add(db)
		exact := new(big.Rat).Add(ra, rb)
		if err == nil {
			if got := sum.rat(); got.Cmp(exact) != 0 {
				t.Fatalf("Add(%q, %q) = %s, want %s", a, b, got.RatString(), exact.RatString())
			}
		} else {
			if !errors.Is(err, ErrOverflow) {
				t.Fatalf("Add(%q, %q) error = %v, want ErrOverflow", a, b, err)
			}
			if _, _, _, aligned := align(da, db); aligned {
				if _, fits := ratToDecimal(exact); fits {
					t.Fatalf("Add(%q, %q) reported overflow for the representable %s",
						a, b, exact.RatString())
				}
			}
			reported++
		}
	}
	if reported == 0 {
		t.Fatal("the generator never produced an addition that overflows")
	}
	t.Logf("checked %d reported additions that genuinely do not fit", reported)
}

// checkAgainstOracle asserts the exact contract of an operation: either it
// returns the oracle value, or it returns an overflow error and the oracle value
// really does not fit the representation.
func checkAgainstOracle(t *testing.T, op, a, b string, exact *big.Rat, got Decimal, err error) {
	t.Helper()
	da, _ := parseFitting(a)
	db, _ := parseFitting(b)
	_, _, _, aligned := align(da, db)
	want, fits := ratToDecimal(exact)

	// This package guarantees "an exact result or an error", not "every
	// representable result is produced": when the operands cannot be brought to a
	// common scale inside an int64 coefficient, Add and Sub report overflow even
	// if the exact result would have been representable after cancelling trailing
	// zeros. Rounding such a result down would be a silent loss of precision,
	// which this package never does. Documented in README under limitations.
	if !aligned {
		if err == nil {
			t.Fatalf("%s(%q, %q) = %s, but the operands cannot be aligned", op, a, b, got)
		}
		if !errors.Is(err, ErrOverflow) {
			t.Fatalf("%s(%q, %q) error = %v, want ErrOverflow", op, a, b, err)
		}
		return
	}
	if err != nil {
		if !errors.Is(err, ErrOverflow) {
			t.Fatalf("%s(%q, %q) error = %v, want ErrOverflow", op, a, b, err)
		}
		if fits {
			t.Fatalf("%s(%q, %q) reported overflow, but %s is representable as %s",
				op, a, b, exact.RatString(), want)
		}
		return
	}
	if !fits {
		t.Fatalf("%s(%q, %q) = %s without an error, but the exact %s does not fit",
			op, a, b, got, exact.RatString())
	}
	if !got.Equal(want) {
		t.Fatalf("%s(%q, %q) = %s, want %s", op, a, b, got, want)
	}
	if got != want {
		t.Fatalf("%s(%q, %q) = %v, want the canonical %v", op, a, b, got, want)
	}
	requireCanonical(t, got, op+"("+a+", "+b+")")
}

// ratToDecimal converts an exact rational to a Decimal when, and only when, the
// value has a finite decimal expansion of at most MaxScale places and a
// coefficient that fits the representation.
func ratToDecimal(r *big.Rat) (Decimal, bool) {
	if r.IsInt() {
		num := r.Num()
		if !num.IsInt64() {
			return Decimal{}, false
		}
		i := num.Int64()
		if i == math.MinInt64 {
			return Decimal{}, false
		}
		return FromInt(i), true
	}

	// The value is a fraction, so find the smallest number of decimal places
	// that writes it exactly: the first scale at which the value times ten to
	// that scale is an integer. The search is exact, because it works on the
	// reduced rational rather than on a denominator that is assumed to be a
	// power of ten.
	ten := big.NewInt(10)
	one := big.NewInt(1)
	for scale := 1; scale <= int(MaxScale); scale++ {
		scaled := new(big.Rat).Mul(r, new(big.Rat).SetInt(new(big.Int).Exp(ten, big.NewInt(int64(scale)), nil)))
		if !scaled.IsInt() {
			continue
		}
		num := scaled.Num()
		if !num.IsInt64() || num.Int64() == math.MinInt64 {
			return Decimal{}, false
		}
		d, err := FromCoefScale(num.Int64(), int8(scale))
		if err != nil {
			return Decimal{}, false
		}
		// FromCoefScale reduces trailing zeros; the scale found here is minimal,
		// so an equal value means the conversion is faithful.
		if got := d.rat(); got.Cmp(r) != 0 {
			return Decimal{}, false
		}
		return d, true
	}
	_ = one
	return Decimal{}, false
}

// rat returns the exact value of d as a big.Rat. It is the bridge to the oracle.
func (d Decimal) rat() *big.Rat {
	return new(big.Rat).SetFrac(big.NewInt(d.coef), pow10Rat(int(d.scale)))
}

// pow10Rat returns 10^n as a big.Int, for n in [0, MaxScale].
func pow10Rat(n int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
}

// parseFitting parses s and reports whether it is representable at all.
func parseFitting(s string) (Decimal, bool) {
	d, err := Parse(s)
	return d, err == nil
}

// parseWithOracle parses s both ways and returns an error when the two disagree
// on acceptance.
func parseWithOracle(t *testing.T, s string) (Decimal, *big.Rat, error) {
	t.Helper()
	d, err := Parse(s)
	oracle, ok := new(big.Rat).SetString(s)
	if !ok {
		t.Fatalf("the oracle rejects %q, which the generator produced", s)
	}
	if err != nil {
		return Decimal{}, nil, err
	}
	return d, oracle, nil
}

func pick(rng *rand.Rand, options []string) string {
	return options[rng.Intn(len(options))]
}

// randomFittingLiteral builds a literal that is guaranteed to be representable.
//
// The literal is derived from a canonical Decimal rather than assembled by
// guesswork: a random coefficient of at most 18 digits is chosen together with a
// scale in [0, MaxScale], and the pair is converted to its exact decimal text.
// The optional exponent is then folded into the scale, so a positive exponent
// can only ever be applied when the coefficient still fits.
func randomFittingLiteral(rng *rand.Rand) string {
	digits := 1 + rng.Intn(18)
	limit := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(digits)), nil)
	coef := new(big.Int).Rand(rng, limit)
	if rng.Intn(2) == 0 {
		coef.Neg(coef)
	}
	if !coef.IsInt64() {
		// The random value hit the top of the range; fall back to a safe one.
		coef.SetInt64(1)
	}

	scale := rng.Intn(int(MaxScale) + 1)
	d, err := FromCoefScale(coef.Int64(), int8(scale))
	if err != nil {
		return "0"
	}
	return d.String()
}

// randomAnyLiteral builds a literal that may or may not be representable: it can
// carry up to 19 significant digits with a value above MaxInt64, or an exponent
// that pushes the scale outside the supported range.
func randomAnyLiteral(rng *rand.Rand) string {
	var sb strings.Builder
	if rng.Intn(2) == 0 {
		sb.WriteByte('-')
	}

	// Up to 19 significant digits in total, which is the largest digit count
	// Parse ever accepts; the value can still exceed MaxInt64, and that is the
	// point of this generator.
	digits := 1 + rng.Intn(19)
	sb.WriteByte(byte('1' + rng.Intn(9)))
	for i := 1; i < digits; i++ {
		sb.WriteByte(byte('0' + rng.Intn(10)))
	}
	if fraction := rng.Intn(21); fraction > 0 && digits+fraction <= 19 {
		sb.WriteByte('.')
		for i := 0; i < fraction; i++ {
			sb.WriteByte(byte('0' + rng.Intn(10)))
		}
	}
	if rng.Intn(3) == 0 {
		sb.WriteByte('e')
		if rng.Intn(2) == 0 {
			// A negative exponent raises the scale, which must stay small enough
			// for the literal to remain a boundary case rather than an
			// out of range one.
			sb.WriteByte('-')
			sb.WriteString(strconv.Itoa(rng.Intn(6)))
		} else {
			sb.WriteString(strconv.Itoa(rng.Intn(25)))
		}
	}
	return sb.String()
}

// TestOracleRound checks Round against a closed-form implementation of
// half-to-even, independent of the one in the package:
//
//	round(x) = floor(x + 1/2) when frac(x + 1/2) != 1/2, and 2*floor(x/2 + 1/4)
//	otherwise. Both are evaluated the same way here: add a half, take the floor,
//	and undo it when the result would be an odd tie break.
//
// Working in units of 10^-target keeps everything integral, so the oracle is
// exact and the comparison is meaningful down to the last digit.
func TestOracleRound(t *testing.T) {
	rng := rand.New(rand.NewSource(20240916))
	for i := 0; i < 20000; i++ {
		s := randomFittingLiteral(rng)
		d, ok := parseFitting(s)
		if !ok {
			continue
		}
		target := int8(rng.Intn(int(MaxScale) + 1))

		got, err := d.Round(target)
		if err != nil {
			// The only failure a fitting value can hit is widening past the
			// coefficient range, which the oracle must agree is impossible.
			if errors.Is(err, ErrOverflow) && int(target) > d.Scale() {
				continue
			}
			t.Fatalf("Round(%q, %d) error = %v", s, target, err)
		}

		want := roundHalfEvenOracle(d.rat(), int(target))
		if got.rat().Cmp(want) != 0 {
			t.Fatalf("Round(%q, %d) = %s, oracle says %s",
				s, target, got.rat().RatString(), want.RatString())
		}
		requireCanonical(t, got, "Round("+s+")")
		if got.Scale() > int(target) {
			t.Fatalf("Round(%q, %d) = %v has scale %d", s, target, got, got.Scale())
		}
	}
}

// roundHalfEvenOracle rounds r to n decimal places using half-to-even, computed
// with exact integer arithmetic.
func roundHalfEvenOracle(r *big.Rat, n int) *big.Rat {
	// 1. Work in units of 10^-n: the value becomes a rational p/q near an
	// integer, and the rounding decision is a comparison against 1/2.
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
	scaled := new(big.Rat).Mul(r, new(big.Rat).SetInt(scale))

	// 2. Split into the floor and the fractional part, both exact.
	floor := new(big.Int).Div(scaled.Num(), scaled.Denom()) // Euclidean: goes towards -inf
	frac := new(big.Rat).Sub(scaled, new(big.Rat).SetInt(floor))

	half := big.NewRat(1, 2)
	switch frac.Cmp(half) {
	case -1:
		// Below the midpoint: the floor is already nearest.
	case 1:
		// Above the midpoint: move away from zero, which for a Euclidean floor
		// means stepping up regardless of sign.
		floor.Add(floor, big.NewInt(1))
	default:
		// Exactly at the midpoint: pick the even neighbour.
		if floor.Bit(0) == 1 {
			floor.Add(floor, big.NewInt(1))
		}
	}
	return new(big.Rat).SetFrac(floor, scale)
}
