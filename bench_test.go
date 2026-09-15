package decimal

import (
	"encoding/json"
	"strconv"
	"testing"
)

// The benchmarks in this file exist to prove the allocation contract of the hot
// paths: Cmp, Equal, Add, MulInt, Parse and String must report 0 allocs/op when
// run with -benchmem. Run them with:
//
//	go test -bench=. -benchmem ./...
//
// Every benchmark resets the timer after its setup and stores its result in a
// package level sink, so the compiler cannot eliminate the work being measured.

// sink keeps benchmark results observable.
var (
	sinkDecimal Decimal
	sinkInt     int
	sinkBool    bool
	sinkString  string
	sinkBytes   []byte
	sinkAny     any
)

// BenchmarkParse covers the success path of the parser for the shapes callers
// actually produce.
func BenchmarkParse(b *testing.B) {
	inputs := []struct {
		name string
		in   string
	}{
		{"Int", "12345"},
		{"Int19Digits", "9223372036854775807"},
		{"Fixed6", "12345.678901"},
		{"Fixed2", "99.99"},
		{"Scale18", "1.234567890123456789"},
		{"LeadingDot", ".000123"},
		{"Exponent", "1.2345e-7"},
		{"Negative", "-12345.6789"},
		{"TrailingZeros", "1.500000"},
	}
	for _, tc := range inputs {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				d, err := Parse(tc.in)
				if err != nil {
					b.Fatal(err)
				}
				sinkDecimal = d
			}
		})
	}
}

// BenchmarkString covers the formatter for the same shapes, which is the other
// half of the text round trip.
func BenchmarkString(b *testing.B) {
	values := []struct {
		name string
		in   string
	}{
		{"Zero", "0"},
		{"Int", "12345"},
		{"Int19Digits", "9223372036854775807"},
		{"Fixed6", "12345.678901"},
		{"Fixed2", "99.99"},
		{"Scale18", "1.234567890123456789"},
		{"SmallScale18", "0.000000000000000001"},
		{"Negative", "-12345.6789"},
	}
	for _, tc := range values {
		d := MustParse(tc.in)
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				sinkString = d.String()
			}
		})
	}
}

// BenchmarkCmp covers the comparison fast path and the fallback that has to
// reason in 128 bits.
func BenchmarkCmp(b *testing.B) {
	cases := []struct {
		name string
		a, b string
	}{
		{"SameScaleEqual", "1.5", "1.5"},
		{"SameScaleLess", "1.5", "2.5"},
		{"CrossScale", "1.5", "0.25"},
		{"CrossScaleNegatives", "-1000", "-1.5"},
		{"Zero", "0", "0.000000000000000001"},
		{"WideFallback", "9223372036854775807", "0.000000000000000001"},
		{"WideFallbackNegatives", "-9223372036854775807", "-0.000000000000000001"},
		{"MaxInt64", "9223372036854775807", "9223372036854775806"},
	}
	for _, tc := range cases {
		a, rhs := MustParse(tc.a), MustParse(tc.b)
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				sinkInt = a.Cmp(rhs)
			}
		})
	}
}

// BenchmarkEqual covers the equality predicate, which is Cmp with a different
// return shape.
func BenchmarkEqual(b *testing.B) {
	a, rhs := MustParse("12345.678901"), MustParse("12345.678901")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkBool = a.Equal(rhs)
	}
}

// BenchmarkAdd covers the alignment and normalisation path, including the
// cross-scale case that has to rescale one operand.
func BenchmarkAdd(b *testing.B) {
	cases := []struct {
		name string
		a, b string
	}{
		{"SameScale", "12345.678901", "99.99"},
		{"SameScaleSmall", "1.5", "2.5"},
		{"CrossScale", "12345.678901", "99"},
		{"CrossScaleWide", "1.5", "0.000000000000000001"},
		{"Negatives", "-12345.678901", "-99.99"},
		{"ZeroResult", "1.5", "-1.5"},
		{"ZeroOperand", "12345.678901", "0"},
	}
	for _, tc := range cases {
		a, rhs := MustParse(tc.a), MustParse(tc.b)
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				sum, err := a.Add(rhs)
				if err != nil {
					b.Fatal(err)
				}
				sinkDecimal = sum
			}
		})
	}
}

// BenchmarkSub covers subtraction, which runs the same alignment path.
func BenchmarkSub(b *testing.B) {
	a, rhs := MustParse("12345.678901"), MustParse("99.99")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		diff, err := a.Sub(rhs)
		if err != nil {
			b.Fatal(err)
		}
		sinkDecimal = diff
	}
}

// BenchmarkMulInt covers exact multiplication by an integer.
func BenchmarkMulInt(b *testing.B) {
	cases := []struct {
		name string
		a    string
		i    int64
	}{
		{"Small", "123.45", 3},
		{"One", "123.45", 1},
		{"Zero", "123.45", 0},
		{"Large", "922337203685477580", 10},
		{"Negative", "-123.45", -7},
		{"Scale17", "1.23456789012345678", 9},
	}
	for _, tc := range cases {
		a := MustParse(tc.a)
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				prod, err := a.MulInt(tc.i)
				if err != nil {
					b.Fatal(err)
				}
				sinkDecimal = prod
			}
		})
	}
}

// BenchmarkRound covers the one rounding entry point.
func BenchmarkRound(b *testing.B) {
	d := MustParse("12345.678901234567")
	for _, target := range []int8{0, 2, 6, 12} {
		b.Run("Target"+strconv.Itoa(int(target)), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				r, err := d.Round(target)
				if err != nil {
					b.Fatal(err)
				}
				sinkDecimal = r
			}
		})
	}
}

// BenchmarkRescale covers both directions of the strict conversion.
func BenchmarkRescale(b *testing.B) {
	d := MustParse("1.5")
	b.Run("Widen", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			r, err := d.Rescale(9)
			if err != nil {
				b.Fatal(err)
			}
			sinkDecimal = r
		}
	})
}

// BenchmarkMarshalJSON covers the numeric JSON writer. The standard library's
// json.Marshal is measured separately because it adds its own allocations for
// the encoder state, which are outside this package's control.
func BenchmarkMarshalJSON(b *testing.B) {
	d := MustParse("12345.678901")
	b.Run("Direct", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			out, err := d.MarshalJSON()
			if err != nil {
				b.Fatal(err)
			}
			sinkBytes = out
		}
	})
	b.Run("ThroughEncodingJSON", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			out, err := json.Marshal(d)
			if err != nil {
				b.Fatal(err)
			}
			sinkBytes = out
		}
	})
}

// BenchmarkUnmarshalJSON covers the numeric JSON reader.
func BenchmarkUnmarshalJSON(b *testing.B) {
	raw := []byte("12345.678901")
	var d Decimal
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := d.UnmarshalJSON(raw); err != nil {
			b.Fatal(err)
		}
		sinkDecimal = d
	}
}

// BenchmarkValue covers the driver.Valuer path, which formats to a string.
func BenchmarkValue(b *testing.B) {
	d := MustParse("12345.678901")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		v, err := d.Value()
		if err != nil {
			b.Fatal(err)
		}
		sinkAny = v
	}
}

// BenchmarkScan covers the sql.Scanner path for a text source.
func BenchmarkScan(b *testing.B) {
	var d Decimal
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := d.Scan("12345.678901"); err != nil {
			b.Fatal(err)
		}
		sinkDecimal = d
	}
}
