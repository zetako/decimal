package decimal_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/zetako/decimal"
)

// ExampleParse shows the accepted forms and the canonical shape a value is
// stored in: "1.0" and "1.50" lose their trailing zeros, and integers never
// carry a scale.
func ExampleParse() {
	for _, s := range []string{"1.0", "1.50", "-0.0", ".5", "5.", "1e3", "0.000000000000000001"} {
		d, err := decimal.Parse(s)
		if err != nil {
			fmt.Println("error:", err)
			continue
		}
		fmt.Printf("Parse(%q) = coef %d, scale %d -> %s\n", s, d.Coef(), d.Scale(), d)
	}

	// Output:
	// Parse("1.0") = coef 1, scale 0 -> 1
	// Parse("1.50") = coef 15, scale 1 -> 1.5
	// Parse("-0.0") = coef 0, scale 0 -> 0
	// Parse(".5") = coef 5, scale 1 -> 0.5
	// Parse("5.") = coef 5, scale 0 -> 5
	// Parse("1e3") = coef 1000, scale 0 -> 1000
	// Parse("0.000000000000000001") = coef 1, scale 18 -> 0.000000000000000001
}

// ExampleParse_errors shows that rejection is explicit: the parser never rounds
// and never truncates, and every failure wraps a sentinel that errors.Is can
// match.
func ExampleParse_errors() {
	for _, s := range []string{"", "1e-19", "1e19", "NaN", "1.2.3", "1_000"} {
		_, err := decimal.Parse(s)
		switch {
		case errors.Is(err, decimal.ErrSyntax):
			fmt.Printf("Parse(%q): syntax error\n", s)
		case errors.Is(err, decimal.ErrScaleOutOfRange):
			fmt.Printf("Parse(%q): needs more than %d decimal places\n", s, decimal.MaxScale)
		case errors.Is(err, decimal.ErrOverflow):
			fmt.Printf("Parse(%q): does not fit an int64 coefficient\n", s)
		default:
			fmt.Printf("Parse(%q): %v\n", s, err)
		}
	}

	// Output:
	// Parse(""): syntax error
	// Parse("1e-19"): needs more than 18 decimal places
	// Parse("1e19"): does not fit an int64 coefficient
	// Parse("NaN"): syntax error
	// Parse("1.2.3"): syntax error
	// Parse("1_000"): syntax error
}

// ExampleAdd shows exact addition across different scales, including the
// canonicalisation of the result: 1.5 + 2 is 3.5, and 0.5 + 0.5 is the integer
// 1 rather than 1.0.
func ExampleDecimal_Add() {
	a := decimal.MustParse("1.5")
	b, err := decimal.FromInt(2)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	sum, err := a.Add(b)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("%s + %s = %s (scale %d)\n", a, b, sum, sum.Scale())

	half := decimal.MustParse("0.5")
	doubled, err := half.Add(half)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("%s + %s = %s (scale %d)\n", half, half, doubled, doubled.Scale())

	// Output:
	// 1.5 + 2 = 3.5 (scale 1)
	// 0.5 + 0.5 = 1 (scale 0)
}

// ExampleAdd_overflow shows that an inexact result is reported rather than
// silently wrapped, and that the error names the cause.
func ExampleDecimal_Add_overflow() {
	big := decimal.MustParse("9223372036854775807")
	one, err := decimal.FromInt(1)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	_, err = big.Add(one)
	fmt.Println(errors.Is(err, decimal.ErrOverflow))

	// Output:
	// true
}

// ExampleCmp shows numerical comparison across scales and signs. The comparison
// is exact, so 0.5 is correctly reported as smaller than 1 even though the
// coefficient of 0.5 is the larger of the two.
func ExampleDecimal_Cmp() {
	a := decimal.MustParse("0.5")
	b, err := decimal.FromInt(1)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println(a.Cmp(b))
	fmt.Println(a.LessThan(b))
	fmt.Println(b.GreaterThan(a))
	fmt.Println(a.Cmp(decimal.MustParse("0.50")))

	// Output:
	// -1
	// true
	// true
	// 0
}

// ExampleCmp_zeroRepresentation shows that every spelling of zero is the same
// number and the same value, because zero has a single representation.
func ExampleDecimal_Cmp_zeroRepresentation() {
	var parsed decimal.Decimal
	for _, s := range []string{"0", "-0", "0.0", "-0.000", "0e10"} {
		d := decimal.MustParse(s)
		if d != decimal.Zero() {
			fmt.Printf("%s is not the canonical zero\n", s)
		}
		parsed = d
	}
	fmt.Println("all spellings canonical:", parsed == decimal.Zero(), parsed.IsZero())

	// Output:
	// all spellings canonical: true true
}

// ExampleDecimal_MarshalJSON shows that a Decimal is written as a bare JSON
// number: it is not quoted, and it does not go through float64, so all 19
// digits of a large value survive.
func ExampleDecimal_MarshalJSON() {
	type invoice struct {
		Total decimal.Decimal `json:"total"`
		Tax   decimal.Decimal `json:"tax"`
	}

	in := invoice{
		Total: decimal.MustParse("1.50"),
		Tax:   decimal.MustParse("9223372036854775807"),
	}

	out, err := json.Marshal(in)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(string(out))

	// Output:
	// {"total":1.5,"tax":9223372036854775807}
}

// ExampleDecimal_UnmarshalJSON shows the accepted wire forms and the deliberate
// refusal of the quoted string form.
func ExampleDecimal_UnmarshalJSON() {
	for _, raw := range []string{"1.5", "1.5e3", "null", `"1.5"`} {
		var d decimal.Decimal
		err := json.Unmarshal([]byte(raw), &d)
		if err != nil {
			fmt.Printf("%-8s -> rejected: numbers only\n", raw)
			continue
		}
		fmt.Printf("%-8s -> %s\n", raw, d)
	}

	// Output:
	// 1.5      -> 1.5
	// 1.5e3    -> 1500
	// null     -> 0
	// "1.5"    -> rejected: numbers only
}

// ExampleDecimal_Round shows half-to-even rounding, the only rounding this
// package performs and only when it is asked for explicitly.
func ExampleDecimal_Round() {
	for _, s := range []string{"2.5", "3.5", "-2.5", "-3.5", "0.5", "1.235", "1.245"} {
		target := int8(0)
		if len(s) > 3 && s[1] == '.' && len(s) == 5 {
			target = 2
		}
		d := decimal.MustParse(s)
		r, err := d.Round(target)
		if err != nil {
			fmt.Println("error:", err)
			continue
		}
		fmt.Printf("%s rounded to %d places = %s\n", d, target, r)
	}

	// Output:
	// 2.5 rounded to 0 places = 2
	// 3.5 rounded to 0 places = 4
	// -2.5 rounded to 0 places = -2
	// -3.5 rounded to 0 places = -4
	// 0.5 rounded to 0 places = 0
	// 1.235 rounded to 2 places = 1.24
	// 1.245 rounded to 2 places = 1.24
}

// ExampleDecimal_Rescale shows the strict counterpart of Round: a conversion
// that cannot preserve the value is refused instead of rounded.
func ExampleDecimal_Rescale() {
	d := decimal.MustParse("1.5")

	widened, err := d.Rescale(4)
	fmt.Printf("Rescale(4) = %s, canonical scale %d, err %v\n", widened, widened.Scale(), err)

	_, err = d.Rescale(0)
	fmt.Println("Rescale(0) loses precision:", errors.Is(err, decimal.ErrOverflow))

	whole := decimal.MustParse("12.00")
	narrowed, err := whole.Rescale(1)
	fmt.Printf("%s.Rescale(1) = %s, err %v\n", whole, narrowed, err)

	// Output:
	// Rescale(4) = 1.5, canonical scale 1, err <nil>
	// Rescale(0) loses precision: true
	// 12.Rescale(1) = 12, err <nil>
}

// ExampleDecimal_MulInt shows exact multiplication by an integer, the case that
// involves no scale arithmetic at all.
func ExampleDecimal_MulInt() {
	price := decimal.MustParse("19.99")

	for _, qty := range []int64{0, 1, 3, 1000} {
		total, err := price.MulInt(qty)
		if err != nil {
			fmt.Println("error:", err)
			continue
		}
		fmt.Printf("%s x %d = %s\n", price, qty, total)
	}

	// Output:
	// 19.99 x 0 = 0
	// 19.99 x 1 = 19.99
	// 19.99 x 3 = 59.97
	// 19.99 x 1000 = 19990
}

// ExampleDecimal_Mul shows that decimal multiplication is exact or it fails:
// there is no rounding policy, so an intermediate that does not fit is divided
// down only where the division is exact, and a value the representation cannot
// hold is refused rather than approximated.
func ExampleDecimal_Mul() {
	price := decimal.MustParse("19.99")
	qty := decimal.MustParse("2.5")

	total, err := price.Mul(qty)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("19.99 x 2.5 =", total)

	// This one passes through 20000000000000000000 at scale 1, which does not fit
	// an int64 coefficient, and loses nothing when that ten is cancelled.
	if big, err := decimal.MustParse("4000000000000000000").Mul(decimal.MustParse("0.5")); err == nil {
		fmt.Println("4000000000000000000 x 0.5 =", big)
	}

	// What is still refused is a value no representation can hold: 1e-19 would
	// need 19 decimal places.
	if _, err := decimal.MustParse("1e-9").Mul(decimal.MustParse("1e-10")); err != nil {
		fmt.Println("refused:", err)
	}

	// Output:
	// 19.99 x 2.5 = 49.975
	// 4000000000000000000 x 0.5 = 2000000000000000000
	// refused: decimal: scale out of range: product needs more than 18 decimal places
}

// ExampleFromInt_overflow shows the single int64 that has no Decimal: its
// magnitude is not negatable, so the constructor reports it rather than handing
// back a value nothing else in the package could use.
func ExampleFromInt_overflow() {
	_, err := decimal.FromInt(math.MinInt64)
	fmt.Println(err)
	fmt.Println(errors.Is(err, decimal.ErrOverflow))

	// Output:
	// decimal: overflow: coefficient is MinInt64, whose magnitude is not negatable
	// true
}

// ExampleFromCoefScale shows the programmatic constructor and the fact that it
// canonicalises whatever it is given.
func ExampleFromCoefScale() {
	for _, c := range []struct {
		coef  int64
		scale int8
	}{{150, 2}, {15, 1}, {1000, 3}, {0, 18}, {1, 18}} {
		d, err := decimal.FromCoefScale(c.coef, c.scale)
		if err != nil {
			fmt.Println("error:", err)
			continue
		}
		fmt.Printf("coef %d scale %d -> %s\n", c.coef, c.scale, d)
	}

	// Output:
	// coef 150 scale 2 -> 1.5
	// coef 15 scale 1 -> 1.5
	// coef 1000 scale 3 -> 1
	// coef 0 scale 18 -> 0
	// coef 1 scale 18 -> 0.000000000000000001
}

// ExampleDecimal_Int64 shows that converting out of the type is exact or it
// fails: a value with a fractional part is not truncated silently.
func ExampleDecimal_Int64() {
	for _, s := range []string{"42", "-42", "42.0", "42.5", "9223372036854775807"} {
		d := decimal.MustParse(s)
		i, ok := d.Int64()
		fmt.Printf("%s -> %d, ok %v\n", d, i, ok)
	}

	// Output:
	// 42 -> 42, ok true
	// -42 -> -42, ok true
	// 42 -> 42, ok true
	// 42.5 -> 0, ok false
	// 9223372036854775807 -> 9223372036854775807, ok true
}

// ExampleDecimal_Value shows the database round trip: the value is stored as its
// canonical text, so the database never sees a binary float.
func ExampleDecimal_Value() {
	d := decimal.MustParse("1234.500")

	v, err := d.Value()
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("Value() = %v (%T)\n", v, v)

	var back decimal.Decimal
	if err := back.Scan("1234.5"); err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("Scan(\"1234.5\") =", back, "equal:", back.Equal(d))

	// Output:
	// Value() = 1234.5 (string)
	// Scan("1234.5") = 1234.5 equal: true
}

// ExampleMaxScale shows the bound this package enforces, and the fact that any
// tighter business limit belongs to the caller.
func ExampleMaxScale() {
	d := decimal.MustParse("0.000000000000000001")
	fmt.Println("MaxScale:", decimal.MaxScale)
	fmt.Println("smallest value:", d, "scale", d.Scale())

	// A caller that only accepts six decimal places checks at its own boundary.
	const businessScale = 6
	fmt.Println("accepted by the business rule:", d.Scale() <= businessScale)

	// Output:
	// MaxScale: 18
	// smallest value: 0.000000000000000001 scale 18
	// accepted by the business rule: false
}
