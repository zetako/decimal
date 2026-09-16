// Package decimal implements a bounded fixed-point decimal type.
//
// The type is a small value type made of an int64 coefficient and an int8
// scale:
//
//	type Decimal struct {
//		coef  int64  // coefficient (carries the sign)
//		scale int8   // number of decimal places
//	}
//
// A Decimal represents the exact mathematical value coef * 10^-scale. Every
// value that fits in that representation is stored exactly: there is no binary
// floating point involved anywhere in parsing, comparison or arithmetic.
//
// # Invariants
//
// Every Decimal handed out by this package (including the zero Decimal, since
// the fields are unexported and never set outside the package) satisfies:
//
//   - The sign lives in Coef; there is no separate sign bit.
//   - Zero has exactly one representation: Coef() == 0 && Scale() == 0, so the
//     literal "-0.0" parses to the canonical zero.
//   - Values are canonical: Scale() == 0 if and only if the value is an
//     integer, and there are no trailing zeros in the fraction ("1.0" becomes
//     1 with scale 0, "1.50" becomes 1.5 with scale 1).
//   - 0 <= Scale() <= MaxScale and math.MinInt64 <= Coef() <= math.MaxInt64.
//
// # Value semantics
//
// Decimal is a value type: no method mutates its receiver, every operation
// returns a fresh Decimal. Comparing two values with == is therefore only
// meaningful as an identity check on the canonical form, not as a numeric test;
// use Equal, Cmp, LessThan or GreaterThan instead (they compare numbers, not
// representations). Decimal contains no pointers, so copying and comparing it
// with == is safe and never panics; just do not read == as "same number".
//
// # Boundaries
//
// MaxScale is 18, the largest number of decimal places for which 10^-18 is
// still representable as a non-zero coefficient. The magnitude of a value is
// bounded by math.MaxInt64 (about 9.22e18) at scale 0, and shrinks by a factor
// of ten for each additional scale unit: at scale 18 the largest magnitude is
// about 9.22. This package deliberately does not know about any caller's
// business rules: if an application only accepts scale <= 6, it must check
// Scale() at its own boundary.
//
// # Errors
//
// Every operation that cannot produce an exact result returns an error instead
// of rounding or truncating silently. The package never rounds by itself; the
// only rounding entry point is Round, which is explicit and half-even. Errors
// wrap the exported sentinels ErrSyntax, ErrOverflow and ErrScaleOutOfRange and
// can be tested with errors.Is. Apart from MustParse, nothing in this package
// panics.
//
// # Exhaustiveness
//
// The guarantee is "an exact result or an error", not "every representable
// result is produced". Add and Sub first bring their operands to a common scale
// inside an int64 coefficient; when that alignment overflows, they report
// ErrOverflow even if the exact sum would have fitted after cancelling trailing
// zeros, because dividing that intermediate down would be a silent loss of
// precision and they never do it implicitly.
//
// Mul is the exception, and it errs on the other side: it cancels the factors of
// ten its intermediate really contains, so it returns every product the
// representation can hold. 4000000000000000000 * 0.5 passes through
// 20000000000000000000 at scale 1 and comes back as the exact 2000000000000000000,
// while 0.0000000001 * 0.000000001 still reports ErrScaleOutOfRange, because
// 1e-19 has no representation at all, and 9223372036854775807 * 2 still reports
// ErrOverflow, because 18446744073709551614 does not fit an int64 coefficient
// even with its one ten cancelled.
//
// # Rounding
//
// The single rounding entry point is Round, which is explicit and half-to-even.
// In particular there is no rounding policy attached to the arithmetic: a caller
// that wants a product at a business scale multiplies exactly and then rounds the
// result with Round, which makes the loss of precision a visible, named step.
//
// # Operations that are deliberately not provided
//
// This is an intentionally minimal type:
//
//   - no arbitrary precision (no big.Int/big.Rat backing);
//   - no division, exponentiation or square root;
//   - no nullable variant;
//   - no JSON output as a quoted string, only bare JSON numbers.
//
// # Allocation behaviour
//
// Cmp, Equal, Add, Sub, MulInt, Mul, Round, Rescale, Parse and the value queries
// perform no heap allocation on the success path ("0 allocs/op"), and Cmp and
// Equal allocate nothing on any path, because they detect an unalignable pair
// with bit arithmetic instead of formatting an error. String performs exactly
// one allocation, the string it returns, and MarshalJSON one, the byte slice it
// returns; both encode into a stack-backed buffer first, so those are the
// minimum for their signatures. See the benchmarks in bench_test.go and the
// assertions in alloc_test.go.
//
// # Example
//
//	a, err := decimal.Parse("1.50")   // 1.5, scale 1
//	if err != nil {
//		return err
//	}
//	b := decimal.FromInt(2)           // 2, scale 0
//	sum, err := a.Add(b)              // 3.5
//	if err != nil {
//		return err
//	}
//	fmt.Println(sum, sum.Cmp(b))      // 3.5 1
package decimal
