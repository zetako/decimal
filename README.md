# decimal

[![CI](https://github.com/zetako/decimal/actions/workflows/ci.yml/badge.svg)](https://github.com/zetako/decimal/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/zetako/decimal.svg)](https://pkg.go.dev/github.com/zetako/decimal)
[![Go version](https://img.shields.io/badge/go-1.22%2B-00ADD8?logo=go)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Dependencies](https://img.shields.io/badge/dependencies-0-brightgreen)](go.mod)
[![Allocations](https://img.shields.io/badge/hot%20paths-0%20allocs%2Fop-brightgreen)](#performance)

A small, bounded fixed-point decimal type for Go: an `int64` coefficient and an
`int8` scale, storing decimal values exactly.

```go
type Decimal struct {
	coef  int64 // coefficient, carries the sign
	scale int8  // number of decimal places
}
// value = coef × 10^-scale
```

No arbitrary precision, no division, no exponentiation. Just exact comparison,
addition, subtraction, multiplication (by a decimal or by an integer), scaling,
rounding when you ask for it, and lossless text/JSON round trips.

- **Zero dependencies.** Standard library only.
- **No `float64` anywhere in parsing or arithmetic.** Values are exact or the
  operation returns an error.
- **No silent rounding, ever.** The only rounding entry point is `Round`, which
  is explicit and half-to-even.
- **No heap allocation** for `Cmp`, `Equal`, `Add`, `Sub`, `MulInt`, `Mul`,
  `Round`, `Rescale`, `Parse`, `Scan` and the value queries. `String` allocates
  exactly once, for the string it returns, which is the minimum for that
  signature: the digits themselves are built in a stack buffer.

## Contents

- [Install](#install)
- [Quick start](#quick-start)
- [Representation and invariants](#representation-and-invariants)
- [API](#api)
- [Precision and range limits](#precision-and-range-limits)
- [JSON behaviour](#json-behaviour)
- [Using it with GORM](#using-it-with-gorm)
- [Error handling](#error-handling)
- [Value semantics](#value-semantics)
- [Differences from shopspring/decimal](#differences-from-shopspringdecimal)
- [Performance](#performance)
- [Testing](#testing)
- [Limitations](#limitations)
- [Versioning](#versioning)
- [Contributing](#contributing)
- [License](#license)

## Install

```sh
go get github.com/zetako/decimal
```

Requires Go 1.22 or newer.

## Quick start

```go
package main

import (
	"encoding/json"
	"fmt"

	"github.com/zetako/decimal"
)

func main() {
	price := decimal.MustParse("19.99")
	qty := int64(3)

	total, err := price.MulInt(qty)
	if err != nil {
		panic(err)
	}

	// Decimal by decimal is exact too, or it returns an error: there is no
	// rounding policy to configure.
	half, err := price.Mul(decimal.MustParse("0.5"))
	if err != nil {
		panic(err)
	}

	tax, err := decimal.MustParse("1.4995").Round(2) // 1.50, half-to-even
	if err != nil {
		panic(err)
	}

	grand, err := total.Add(tax)
	if err != nil {
		panic(err)
	}

	fmt.Println(half)                         // 9.995
	fmt.Println(grand)                        // 61.47
	fmt.Println(grand.GreaterThan(total))     // true
	fmt.Println(grand.Cmp(decimal.MustParse("61.470"))) // 0

	out, _ := json.Marshal(grand)
	fmt.Println(string(out))                  // 61.47, a bare number
}
```

## Representation and invariants

A `Decimal` represents the exact mathematical value `coef × 10^-scale`. Every
value this package hands out satisfies all of the following. The fields are
unexported and are never set outside the package, so a value obtained from any
constructor, parser or operation always complies:

| # | Invariant |
|---|-----------|
| 1 | The sign lives in the coefficient. There is no separate sign bit. |
| 2 | Zero has exactly one representation: `coef == 0 && scale == 0`. `"-0.0"` parses to the canonical zero. |
| 3 | Values are canonical: no trailing zeros in the fraction, and `Scale() == 0` if and only if the value is an integer. `"1.0"` becomes `coef=1, scale=0`; `"1.50"` becomes `coef=15, scale=1`. |
| 4 | Every operation normalises its result, so `0.5 + 0.5` is the integer `1`, not `1.0`. |
| 5 | `0 <= Scale() <= MaxScale`, with `MaxScale == 18`. |
| 6 | `|Coef()| <= math.MaxInt64`. `math.MinInt64` is never a coefficient, because its magnitude would not be negatable. |

Because the representation is canonical, `==` on two `Decimal` values means "same
representation", which for canonical values also means "same number". Still,
**compare numbers with `Cmp`/`Equal`**, not with `==`: it states the intent and
does not depend on the canonical form.

The package deliberately does **not** encode any caller's business scale limit.
If an application accepts at most 6 decimal places, it checks `Scale() <= 6` at
its own boundary. `MaxScale` is a property of the representation, not a policy.

## API

### Construction and parsing

| Function | Notes |
|---|---|
| `Parse(string) (Decimal, error)` | Strict, exact, allocation free. Never rounds. |
| `MustParse(string) Decimal` | Panics on invalid input. For tests and constant initialisation only. |
| `FromInt(int64) Decimal` | Exact, scale 0. |
| `FromCoefScale(coef int64, scale int8) (Decimal, error)` | Canonicalises its input; rejects a scale outside `[0, MaxScale]`. |
| `Zero() Decimal` | The canonical zero, identical to the zero value. |

#### Accepted grammar

```
[+-]? digits ['.' digits?] [eE [+-]? digits]
[+-]? '.' digits  [eE [+-]? digits]
```

Accepted: `0`, `-0.0`, `1`, `+1`, `1.50`, `.5`, `5.`, `1e3`, `1E+3`, `1.5e-1`,
`0.000000000000000001`, `9223372036854775807`, `9223372036854775807e-18`.

Rejected with `ErrSyntax`: the empty string, surrounding or embedded whitespace
(`" 1"`, `"1 "`, `"1 000"`), digit separators (`"1_000"`), `NaN`, `Inf`,
`Infinity`, hexadecimal, a second decimal point (`"1.2.3"`), a bare sign (`"+"`),
a bare point (`"."`), and a dangling exponent (`"1e"`, `"1e+"`). Non-ASCII digits
are rejected too.

Rejected with `ErrScaleOutOfRange`: input needing more than 18 fractional digits,
such as `"1e-19"` or `"0.0000000000000000001"`.

Rejected with `ErrOverflow`: a coefficient that does not fit an `int64`, such as
`"9223372036854775808"`, `"9999999999999999999"` or `"1e19"`.

Parsing never rounds: `"1e-19"` is an error, not a rounded `0` or `1e-18`. A
leading `+` is accepted, and `-0.0` normalises to the single canonical zero.

### Queries

| Method | Notes |
|---|---|
| `IsZero() bool` | |
| `IsInt() bool` | Equivalent to `Scale() == 0` for canonical values. |
| `Sign() int` | `-1`, `0` or `+1`. |
| `Scale() int` | Always in `[0, MaxScale]`. |
| `Coef() int64` | The value is `Coef() × 10^-Scale()`. |
| `Int64() (int64, bool)` | `true` only when the scale is 0. Never truncates. |
| `Float64() float64` | **Inexact, for display only.** Never use it to decide anything. |

### Comparison

`Cmp(Decimal) int`, `Equal(Decimal) bool`, `LessThan(Decimal) bool`,
`GreaterThan(Decimal) bool`. Exact across scales and signs, allocation free,
including on the fallback path that reasons in 128 bits.

### Arithmetic

Every operation is exact or it fails. None of them rounds.

| Method | Notes |
|---|---|
| `Neg() Decimal` | |
| `Abs() Decimal` | |
| `Add(Decimal) (Decimal, error)` | Aligns scales, then adds with explicit overflow checks. |
| `Sub(Decimal) (Decimal, error)` | |
| `MulInt(int64) (Decimal, error)` | Exact; fails only when the coefficient product leaves the `int64` range. |
| `Mul(Decimal) (Decimal, error)` | Exact; cancels the tens of the intermediate product, so it fails only for a value no representation can hold. |
| `Rescale(int8) (Decimal, error)` | Widening fails if the coefficient would overflow; narrowing fails if it would lose precision. |
| `Round(int8) (Decimal, error)` | The one explicit rounding point: half-to-even. |

`Rescale` is the strict counterpart of `Round`: `MustParse("1.5").Rescale(0)`
fails, while `MustParse("1.5").Round(0)` returns `2`. Both return canonical
values, so a returned `Scale()` may be smaller than the scale requested, because
rescaling `12.5` to scale 3 gives the canonical `12.5` at scale 1 rather than
`12.500`.

### Serialisation

| Method | Notes |
|---|---|
| `String() string` | Canonical plain notation, no exponent, integers without a point, zero as `"0"`. |
| `MarshalJSON()` / `UnmarshalJSON()` | **Bare JSON numbers only.** |
| `MarshalText()` / `UnmarshalText()` | Identical to `String` / `Parse`. |
| `Value()` / `Scan()` | `driver.Valuer` / `sql.Scanner` over the canonical text. |

## Precision and range limits

The coefficient is an `int64` (up to 19 significant digits) and the scale is
bounded at 18:

| | Value |
|---|---|
| Largest magnitude | `9223372036854775807` (`≈ 9.22e18`) at scale 0 |
| Largest magnitude at scale 18 | `9.223372036854775807` |
| Smallest non-zero magnitude | `1e-18` (`0.000000000000000001`) |
| Significant digits | 19, minus the scale shift |
| Exact decimal places | up to 18 |

A value is representable when its coefficient, after trailing zeros are
removed, fits in an `int64` with at most 18 decimal places. `MaxScale` is 18
because `10^-18` is the smallest non-zero magnitude still reachable: a
coefficient of 1 at scale 19 would no longer be a canonical `int64` digit
pattern.

There is no rounding on the way in or out of the type, so a value that does not
fit is an error rather than a silent loss.

## JSON behaviour

`MarshalJSON` writes a **bare JSON number**, never a quoted string:

```go
type invoice struct {
	Total decimal.Decimal `json:"total"`
}
json.Marshal(invoice{Total: decimal.MustParse("1.50")})
// {"total":1.5}
```

`UnmarshalJSON` accepts a bare JSON number, including exponent notation, and
`null` (which yields the canonical zero):

```go
json.Unmarshal([]byte("1.5"),     &d) // 1.5
json.Unmarshal([]byte("1.5e3"),   &d) // 1500
json.Unmarshal([]byte("null"),    &d) // 0
json.Unmarshal([]byte(`"1.5"`),   &d) // error wrapping ErrSyntax
```

**Quoted strings are rejected**, and this is deliberate. The type writes numbers
and only numbers, so accepting `"1.5"` would let a client believe the two wire
forms are interchangeable when the producer is expected to emit the first one.
The error message says so explicitly. Use `UnmarshalText` when the input really
is text, for instance from a configuration file or a database column.

Decoding never goes through `float64`, so all 19 digits survive:

```go
json.Marshal(decimal.MustParse("9223372036854775807"))
// 9223372036854775807, exactly
```

## Using it with GORM

`Decimal` works as a GORM v2 field type with no wrapper and no hook, because it
already implements `driver.Valuer` and `sql.Scanner`:

```go
type Invoice struct {
	ID    uint            `gorm:"primarykey"`
	Total decimal.Decimal
}

db.AutoMigrate(&Invoice{})
// CREATE TABLE `invoices` (`id` integer PRIMARY KEY AUTOINCREMENT,`total` text)

db.Create(&Invoice{Total: decimal.MustParse("30.20")})

var got Invoice
db.First(&got)               // got.Total is exactly 30.2
```

`AutoMigrate` maps the field to a nullable `text` column, because GORM has no
numeric type mapping for an unknown struct and falls back to text. That is also
the configuration you want: the exact decimal text reaches the database, so
`30.20`, `0.000000000000000001` and `9223372036854775807` all round-trip
unchanged. The zero value is stored as the text `0`, not as SQL `NULL`. A pointer
field (`*decimal.Decimal`) maps to a nullable column: a nil pointer is written as
`NULL`, and `NULL` reads back as either the canonical zero in a value field or nil
in a pointer field.

**One thing to get right: keep the column text-typed.** `database/sql` tests its
own fast paths for `string`, `[]byte`, `int64` and `float64` *before* it calls
`Scanner.Scan`, so this type only sees values that the driver reports as text.
A column with numeric affinity can therefore come back as a `float64`, and this
type refuses that with

```
decimal: cannot scan float64 into Decimal, expected string, []byte or nil
```

That refusal is deliberate: by the time a value is a `float64`, the exact decimal
is already gone, and accepting it would hide the loss. It is a loud failure
instead of a silent rounding, which is the whole point of the type. It also means
the failure is a property of the *column*, not of GORM, and it differs per
database and driver:

| Column | Returned to `Scan` | Result |
|---|---|---|
| `TEXT` (the default) | `string` | exact round trip |
| `DECIMAL` / `NUMERIC` on SQLite | `float64` or `int64` | refused |
| `DECIMAL(19,6)` on MySQL, PostgreSQL | `[]byte` | exact round trip |

So on SQLite, let the default `TEXT` column stand rather than "improving" it to
`DECIMAL(19,6)`: SQLite gives numeric columns numeric affinity and hands the
value back as a `float64`. On MySQL and PostgreSQL a native `DECIMAL` column is
fine, because those drivers return its bytes. If you want a specific column type
across dialects, add the per-dialect hook on a thin wrapper:

```go
type Amount struct{ decimal.Decimal }

func (a Amount) Value() (driver.Value, error) { return a.Decimal.Value() }
func (a *Amount) Scan(src any) error          { return a.Decimal.Scan(src) }

func (Amount) GormDBDataType(db *gorm.DB, field *schema.Field) string {
	if db.Dialector.Name() == "sqlite" {
		// SQLite gives a numeric column numeric affinity and hands the value
		// back as a float64, which Scan refuses. Keep TEXT there.
		return "text"
	}
	return "DECIMAL(19,6)"
}
```

The wrapper is only needed when you want a non-default column type. For the
common case, the plain `decimal.Decimal` field is enough.

Be careful with that hook, because it is the easiest way to break a working
setup: returning `DECIMAL(19,6)` unconditionally, on a `sqlite` dialector,
produces a column that `Create` writes and `First` cannot read back with
`decimal: cannot scan float64 into Decimal, expected string, []byte or nil`. The
safest rule is therefore **`text` everywhere on SQLite**, reached through the
default column type so that no hook is involved at all.

The underlying reason is worth stating precisely, because it is not a property of
the column name alone. `database/sql` resolves the destination before it ever
calls `Scan`, and it does so by the *Go type the driver returns*, which SQLite
picks from the storage class of the value actually stored:

| What the driver returns | Who handles it | Outcome |
|---|---|---|
| `string` | `Scanner.Scan` | parsed exactly |
| `[]byte` | `Scanner.Scan` | parsed exactly |
| `float64` | `database/sql` itself | refused by this type |
| `int64` | `database/sql` itself | refused by this type |

That is why the same `DECIMAL(19,6)` column can read back intact in one program
and fail in another: in the verification runs, the 19-digit value was stored as
an integer storage class, which `go-sqlite3` returned as `uint64` and a raw
`database/sql` read handled via a `[]byte` conversion, while gorm scanning the
same column into a `decimal.Decimal` field was handed a `float64` and failed. A
`text` column, by contrast, always returns `string`. Verified against
`gorm.io/gorm` v1.31.2, `gorm.io/driver/sqlite` v1.6.0 and `go-sqlite3`
v1.14.22.

The same reasoning applies outside GORM, to any `database/sql` code: this type is
safe on a text column and refuses a numeric one.

## Error handling

Three exported sentinels classify every rejection, and all three work with
`errors.Is`:

| Sentinel | Meaning |
|---|---|
| `ErrSyntax` | The input is not a decimal literal. |
| `ErrScaleOutOfRange` | The value would need more than `MaxScale` decimal places. |
| `ErrOverflow` | The value or an intermediate result does not fit an `int64` coefficient. |

`ErrEmptyString` is returned by `UnmarshalText` for empty input, as the
`encoding.TextUnmarshaler` contract requires.

Error messages quote the offending input and name the reason:

```
decimal: scale out of range "1e-19": more than 18 fractional digits
decimal: overflow "1e19": value is too large: an integer coefficient of at most 19 digits is required
decimal: invalid syntax "1_000": unexpected character '_' (0x5f)
```

No exported function panics except `MustParse`, whose name says so. No path
truncates or rounds silently.

## Value semantics

`Decimal` is an immutable value type: no method modifies its receiver and every
operation returns a new value. It contains no pointers, so copying is cheap and
comparing with `==` is safe and never panics. A `Decimal` is safe for concurrent
reads; a variable being written needs the usual external synchronisation.

## Differences from shopspring/decimal

[shopspring/decimal](https://github.com/shopspring/decimal) is a mature
arbitrary-precision library. This package is deliberately smaller and stricter.
The differences that matter:

| Aspect | This package | shopspring/decimal |
|---|---|---|
| Representation | `int64` + `int8`, fixed 16 bytes | `*big.Int` + `int32`, arbitrary precision |
| JSON output | Bare number only | Quoted string by default (`MarshalJSONWithoutQuotes` opts out) |
| JSON input | Bare number or `null`; quoted strings rejected | Accepts quoted strings and numbers |
| Division, power, sqrt | **Not provided** | Provided |
| Decimal × decimal | **Provided** and exact whenever the value is representable | Provided, with a rounding policy |
| Rounding | Only in `Round`, half-to-even | Rounding modes across arithmetic operations |
| Overflow | Explicit error | Effectively unbounded |
| Precision loss | Impossible except in `Round` | Possible through division and rounding modes |
| Comparisons | `Cmp`, `Equal`, `LessThan`, `GreaterThan` | `Cmp`, `Equal`, `LessThan`, `GreaterThan` |
| Allocations | 0 for `Cmp`/`Add`/`Parse`; 1 for `String` | Heap-allocated `big.Int` per value |
| Nullable variant | Not provided | `NullDecimal` provided |

If you need arbitrary precision, division, or a nullable variant, use
shopspring/decimal. If you need a small, allocation-free value type that cannot
lose precision by accident, this one is a better fit.

## Performance

Measured on an Apple M3 (darwin/arm64) with `go test -bench=. -benchmem`.
Timings are rounded to three significant figures; allocation counts are exact,
because they are the contract rather than a measurement.

| Operation | ns/op | B/op | allocs/op |
|---|---|---|---|
| `Parse("12345.678901")` | 15.7 | 0 | **0** |
| `Parse("9223372036854775807")` | 20.6 | 0 | **0** |
| `String()` (zero) | 5.1 | 0 | **0** |
| `String()` (19 digits) | 20.1 | 24 | 1 |
| `Cmp` same scale | 1.0 | 0 | **0** |
| `Cmp` cross scale | 3.0 | 0 | **0** |
| `Cmp` 128-bit fallback | 6.5 | 0 | **0** |
| `Equal` | 1.0 | 0 | **0** |
| `Add` same scale | 3.9 | 0 | **0** |
| `Add` cross scale | 3.8 | 0 | **0** |
| `MulInt` | 2.4 | 0 | **0** |
| `Mul` | 2.4 | 0 | **0** |
| `Round` | 2.7 | 0 | **0** |
| `MarshalJSON` (direct) | 18.8 | 16 | 1 |
| `UnmarshalJSON` (direct) | 25.5 | 16 | 1 |
| `json.Marshal` (through the encoder) | 118 | 64 | 4 |
| `Scan` from a string | 17.4 | 0 | **0** |

Notes on the rows that are not zero:

- `String` performs exactly one allocation, the string it returns. Its digit
  encoding is stack backed, and a method returning a string cannot do better.
- `MarshalJSON` performs exactly one allocation, the returned byte slice, for
  the same reason. The `json.Marshal` row adds the standard encoder's own
  allocations, which are outside this package's control.
- `UnmarshalJSON` and `UnmarshalText` take a `[]byte` and report one allocation:
  the `[]byte`-to-`string` conversion their interface forces. `Parse` on the same
  text reports 0.

These numbers are asserted, not just claimed: `alloc_test.go` fails the build if
any hot path starts allocating.

## Testing

```sh
go build ./...
go vet ./...
go test ./... -race
go test -bench=. -benchmem ./...
```

The suite contains:

- table-driven boundary tests: `math.MaxInt64` and its neighbours, scale 0 and
  18, `±0`, `".5"`, `"5."`, `"1e3"`, `"1E+3"`, trailing zeros, 19-digit
  literals, over-long fractions and malformed input;
- a differential test against `math/big.Rat` as a **test-only** oracle: tens of
  thousands of random and systematic literal pairs checked through `Parse`,
  `Cmp`, `Add`, `Sub` and `Mul`, including a check that every reported failure
  really is a result the operation cannot construct;
- a second oracle for `Round`, a closed-form half-to-even implementation built
  from exact integer arithmetic, checked against the method on 20 000 random
  values;
- fuzz targets for parsing, arithmetic, rounding, rescaling, JSON and the
  database round trip: no panics, invariant preservation, and idempotent
  round trips;
- allocation tests pinning the counts in the table above, on every hot path
  and on the failure paths too;
- godoc examples for `Parse`, `Add`, `Cmp`, `MarshalJSON`, `UnmarshalJSON`,
  `Round`, `Rescale`, `MulInt`, `Mul`, `FromCoefScale`, `Int64`, `Value` and
  `MaxScale`.

## Limitations

These are deliberate, and documented rather than hidden:

1. **No rounding policy on multiplication.** `Mul` is exact or it fails: it
   rounds nothing, and it divides an intermediate down only where the division is
   exact, so it returns every product the representation can hold
   (`4000000000000000000 * 0.5` is `2000000000000000000`) and refuses the rest
   (`9223372036854775807 * 2` is an error). A caller that wants a product at a
   business scale multiplies exactly and then calls `Round`, which makes the loss
   of precision a visible, named step.
2. **No division, power or square root.**
3. **No arbitrary precision.** Values outside the `int64`/scale-18 window are
   errors, not approximations.
4. **Alignment can reject a representable sum.** `Add` and `Sub` first bring
   their operands to a common scale inside an `int64` coefficient. When that
   step overflows, the operation reports `ErrOverflow` even if the exact result
   would have fitted after cancelling trailing zeros. The guarantee is "an exact
   result or an error", never "every representable result is produced", because
   the alternative would be to round the result down, and this package never
   rounds silently.
5. **Only 18 decimal places.** Not a business limit: callers enforce their own
   on top of this one.
6. **Quoted JSON is neither produced nor accepted.**
7. **`Scan` accepts text only** (a `string`, `[]byte` or `nil`). A `float64` from
   a driver is refused rather than converted, because by then the decimal value
   is already gone.

## Versioning

The latest release is **v0.1.1**. This is pre-1.0: the module follows semantic
versioning, so breaking changes bump the minor version (`v0.1.1` → `v0.2.0`)
rather than the patch version, and the API may change until `v1.0.0`. Pin a
version in `go.mod` if you need stability.

## Contributing

Issues and pull requests are welcome. Before you open one, please run the full
acceptance command:

```sh
gofmt -l . && go build ./... && go vet ./... && go test ./... -race && go test -bench=. -benchmem ./...
```

`AGENTS.md` documents the invariants this package must keep, the zero-allocation
contract, and where each kind of test belongs. Changes to a hot path have to keep
`alloc_test.go` passing, and changes to behaviour need the test that would catch
them being wrong.

### Commit messages

**Every commit in this repository follows the gitmoji format, stated in English —
without exception:**

```
<gitmoji> <one-sentence description>
```

- The emoji *is* the type. There is no `feat:`/`fix:`/`docs:` prefix and no
  `type(scope):` form.
- The description is a single complete sentence in English, not a `topic: detail`
  pair, and it does not end with a period.
- Pick the most representative emoji; if a change spans unrelated concerns, split
  it into several commits rather than writing one vague message.

| gitmoji | Use for |
|---|---|
| ✨ | new feature |
| 🐛 | bug fix |
| ✅ | tests fixed or added, now passing |
| 🔒 | credentials, security |
| 👷 | build, CI configuration |
| 📝 | documentation |
| ♻️ | refactor |
| ⚡ | performance |
| 🔧 | tooling or configuration tweaks |
| 🚀 | deployment |
| 🗑️ | removing code or files |

For example:

```
✨ Implement half-to-even rounding in Decimal.Round
🐛 Fix rounding direction for negative coefficients
✅ Add big.Rat differential tests for cross-scale comparison
📝 Document the contributor conventions in AGENTS.md and README
⚡ Remove a redundant branch from the parse hot path
```

## License

MIT. See [LICENSE](LICENSE).
