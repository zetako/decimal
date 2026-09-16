package decimal

import (
	"math"
	"testing"
)

// The allocation contract, proven rather than asserted in prose.
//
// The task this package was written to requires Cmp, Equal, Add, Sub, MulInt,
// Mul, Parse and String to be allocation free. Two clarifications are recorded
// here, because "0 allocs/op" would otherwise be a claim nobody can check:
//
//   - String has to return a string, so it performs exactly one allocation: the
//     result itself. Its digit encoding is stack backed. It reports 1 alloc/op,
//     which is the minimum possible for a method with that signature.
//   - UnmarshalJSON takes a []byte and must convert it to a string to parse it,
//     so it reports 1 alloc/op. That conversion, not the parser, is the cost:
//     Parse on the same text in a string reports 0.
//
// This test is the regression guard for all of it.

// allocsPerOp measures the heap allocations of f.
func allocsPerOp(f func()) float64 {
	return testing.AllocsPerRun(2000, f)
}

// TestNoAllocationHotPaths asserts the exact allocation count of every operation
// whose cost a caller can care about.
func TestNoAllocationHotPaths(t *testing.T) {
	d := MustParse("12345.678901")
	rhs := MustParse("99.99")
	big18 := MustParse("4000000000000000000")
	half := MustParse("0.5")
	raw := []byte("12345.678901")
	var out Decimal

	tests := []struct {
		name string
		want float64
		f    func()
	}{
		// Required to be allocation free.
		{"Cmp same scale", 0, func() { sinkInt = d.Cmp(rhs) }},
		{"Cmp cross scale", 0, func() { sinkInt = d.Cmp(MustParse("0.25")) }},
		{"Cmp wide fallback", 0, func() {
			sinkInt = MustParse("9223372036854775807").Cmp(MustParse("0.000000000000000001"))
		}},
		{"Equal", 0, func() { sinkBool = d.Equal(rhs) }},
		{"LessThan", 0, func() { sinkBool = d.LessThan(rhs) }},
		{"GreaterThan", 0, func() { sinkBool = d.GreaterThan(rhs) }},
		{"Add", 0, func() { sinkDecimal, sinkErr = d.Add(rhs) }},
		{"Sub", 0, func() { sinkDecimal, sinkErr = d.Sub(rhs) }},
		{"Neg", 0, func() { sinkDecimal = d.Neg() }},
		{"Abs", 0, func() { sinkDecimal = d.Abs() }},
		{"MulInt", 0, func() { sinkDecimal, sinkErr = d.MulInt(3) }},
		{"Mul", 0, func() { sinkDecimal, sinkErr = d.Mul(rhs) }},
		// The reduced path is the other half of Mul: it runs the 128-bit loop and
		// must stay allocation free as well.
		{"Mul reduced", 0, func() { sinkDecimal, sinkErr = big18.Mul(half) }},
		{"Rescale", 0, func() { sinkDecimal, sinkErr = d.Rescale(9) }},
		{"Round", 0, func() { sinkDecimal, sinkErr = d.Round(2) }},
		{"Parse", 0, func() { sinkDecimal, sinkErr = Parse("12345.678901") }},
		{"Parse 19 digits", 0, func() { sinkDecimal, sinkErr = Parse("9223372036854775807") }},
		{"Parse exponent", 0, func() { sinkDecimal, sinkErr = Parse("1.2345e-7") }},
		{"Scan string", 0, func() { sinkErr = out.Scan("12345.678901") }},
		{"IsZero", 0, func() { sinkBool = d.IsZero() }},
		{"IsInt", 0, func() { sinkBool = d.IsInt() }},
		{"Sign", 0, func() { sinkInt = d.Sign() }},
		{"Scale", 0, func() { sinkInt = d.Scale() }},
		{"Coef", 0, func() { sinkInt64 = d.Coef() }},
		{"Int64", 0, func() { sinkInt64, sinkBool = MustParse("42").Int64() }},
		{"Zero", 0, func() { sinkDecimal = Zero() }},
		{"FromInt", 0, func() { sinkDecimal, sinkErr = FromInt(42) }},
		{"FromCoefScale", 0, func() { sinkDecimal, sinkErr = FromCoefScale(150, 2) }},
		{"Float64", 0, func() { sinkFloat = d.Float64() }},

		// Required to be allocation free, except for the result itself.
		{"String", 1, func() { sinkString = d.String() }},
		{"String 19 digits", 1, func() { sinkString = MustParse("9223372036854775807").String() }},
		{"String scale 18", 1, func() { sinkString = MustParse("0.000000000000000001").String() }},

		// Byte slice entry points pay for the []byte to string conversion.
		{"UnmarshalJSON", 1, func() { sinkErr = out.UnmarshalJSON(raw) }},
		{"UnmarshalText", 1, func() { sinkErr = out.UnmarshalText(raw) }},
		{"MarshalJSON", 1, func() { sinkBytes, sinkErr = d.MarshalJSON() }},
		{"MarshalText", 1, func() { sinkBytes, sinkErr = d.MarshalText() }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Warm up, so that any one time initialisation is not counted.
			tc.f()
			if got := allocsPerOp(tc.f); got != tc.want {
				t.Errorf("%s allocates %.1f times per op, want %.1f", tc.name, got, tc.want)
			}
		})
	}
}

// errorAllocBound is the ceiling used for failure paths. It is deliberately
// loose: the point is to catch a regression that makes failures pathologically
// expensive, not to pin the exact size of an error value, which the race
// detector and future changes to the message wording both move. The strict
// counts live in TestNoAllocationHotPaths, on the paths that must not allocate at
// all.
const errorAllocBound = 8

// TestErrorPathAllocations records the cost of failure separately, because a
// failure has to describe itself: the error necessarily quotes the offending
// input, so it cannot be free.
//
// The one exception is Cmp, which detects an unalignable pair with bit
// arithmetic and therefore never formats anything on any path, on any build.
func TestErrorPathAllocations(t *testing.T) {
	tests := []struct {
		name     string
		maxAlloc float64
		f        func()
	}{
		{"Cmp over the wide path", 0, func() {
			sinkInt = MustParse("9223372036854775807").Cmp(MustParse("-0.000000000000000001"))
		}},
		{"Equal over the wide path", 0, func() {
			sinkBool = MustParse("9223372036854775807").Equal(MustParse("-0.000000000000000001"))
		}},
		{"MulInt overflow", errorAllocBound, func() { sinkDecimal, sinkErr = MustParse("9223372036854775807").MulInt(2) }},
		{"Mul overflow", errorAllocBound, func() {
			sinkDecimal, sinkErr = MustParse("9223372036854775807").Mul(mustFromInt(2))
		}},
		{"Mul scale out of range", errorAllocBound, func() {
			sinkDecimal, sinkErr = MustParse("1e-9").Mul(MustParse("1e-10"))
		}},
		{"Add alignment overflow", errorAllocBound, func() {
			sinkDecimal, sinkErr = MustParse("9223372036854775807").Add(MustParse("0.000000000000000001"))
		}},
		{"Add sum overflow", errorAllocBound, func() {
			sinkDecimal, sinkErr = MustParse("9223372036854775807").Add(mustFromInt(1))
		}},
		{"FromInt MinInt64", errorAllocBound, func() { sinkDecimal, sinkErr = FromInt(math.MinInt64) }},
		{"FromCoefScale MinInt64", errorAllocBound, func() {
			sinkDecimal, sinkErr = FromCoefScale(math.MinInt64, 0)
		}},
		{"Rescale lost precision", errorAllocBound, func() { sinkDecimal, sinkErr = MustParse("1.5").Rescale(0) }},
		{"Round out of range", errorAllocBound, func() { sinkDecimal, sinkErr = MustParse("1.5").Round(19) }},
		{"Parse syntax error", errorAllocBound, func() { sinkDecimal, sinkErr = Parse("oops") }},
		{"Parse scale error", errorAllocBound, func() { sinkDecimal, sinkErr = Parse("1e-19") }},
		{"Parse overflow", errorAllocBound, func() { sinkDecimal, sinkErr = Parse("1e19") }},
		{"Scan wrong type", errorAllocBound, func() { sinkErr = sinkDecimal.Scan(1.5) }},
		{"UnmarshalJSON quoted", errorAllocBound, func() { sinkErr = sinkDecimal.UnmarshalJSON([]byte(`"1.5"`)) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.f()
			got := allocsPerOp(tc.f)
			if got > tc.maxAlloc {
				t.Errorf("%s allocates %.1f times per op, want at most %.1f", tc.name, got, tc.maxAlloc)
			}
		})
	}
}

// sinkErr keeps errors observable in benchmarks and allocation tests.
var sinkErr error

// sinkInt64 and sinkFloat keep the remaining value results observable.
var (
	sinkInt64 int64
	sinkFloat float64
)
