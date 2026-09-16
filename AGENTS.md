# AGENTS.md

Guidance for agents and humans working on `github.com/zetako/decimal`.

This package is deliberately small and deliberately strict. Most of the rules
below exist because a decimal type is easy to make subtly wrong: a rounding step
that looks harmless, a comparison that rescales one side, or a `float64` that
sneaks into a parse path all produce wrong money silently. Read this file before
changing anything; the invariants section is the part that matters most.

## Repository layout

| File | Responsibility |
|---|---|
| `doc.go` | Package godoc: representation, invariants, boundaries, errors, non-features |
| `decimal.go` | The `Decimal` type, `MaxScale`, constructors, `normalize`, value queries, `Float64` |
| `parse.go` | The single strict parser used by every text entry point |
| `arith.go` | `Add`, `Sub`, `MulInt`, `Mul`, `Rescale`, `Round` and the checked arithmetic helpers |
| `compare.go` | `Cmp`, the predicates, `align`, and the 128-bit comparison fallback |
| `format.go` | The canonical text encoder |
| `errors.go` | Sentinel errors and the message builders |
| `json.go` | `MarshalJSON` / `UnmarshalJSON` / `MarshalText` / `UnmarshalText` |
| `sql.go` | `driver.Valuer` / `sql.Scanner` |
| `invariants_test.go` | Test-only invariant helpers, shared by every test file |
| `oracle_test.go` | Differential tests against `math/big.Rat` and a closed-form `Round` oracle |
| `fuzz_test.go` | Fuzz targets |
| `alloc_test.go` | The machine-checked allocation contract |
| `example_test.go` | Godoc examples, which are also runnable tests |
| `bench_test.go` | Benchmarks backing the README performance table |

Source files are split by responsibility, not by type. Keep them that way: a new
README of the type's behaviour belongs in `doc.go`, a new operation belongs in
`arith.go` (or `compare.go` if it only orders values).

## Non-negotiable invariants

Every value reachable through this package satisfies all of these. A change that
breaks one is a bug, not a trade-off.

1. `value = coef × 10^-scale`, with `coef` an `int64` and `scale` an `int8`.
2. The sign lives in the coefficient. There is no separate sign field.
3. Zero has exactly one representation: `coef == 0 && scale == 0`. Every
   spelling of zero (`"-0.0"`, `"-0e5"`, `"0.000"`) collapses to it.
4. Values are canonical: no trailing zeros in the fraction, and `scale == 0` if
   and only if the value is an integer. `"1.0"` → `{1, 0}`, `"1.50"` → `{15, 1}`.
5. Every operation returns a canonical value. `normalize` is the only function
   that may produce the canonical form, and every constructor and every
   arithmetic result must pass through it.
6. `0 <= scale <= MaxScale`, and `MaxScale == 18`.
7. `coef != math.MinInt64`. Its magnitude would not be negatable, so it is not a
   usable coefficient. Every entry point refuses it: `FromInt` and
   `FromCoefScale` report `ErrOverflow`, `Parse` rejects the literal, and `Add`,
   `Mul` and `MulInt` check for it explicitly.

`invariants_test.go` provides `isCanonical` and `requireCanonical`; assert with
them rather than re-deriving the rules in a new test.

## Hard rules

- **No third-party dependencies.** Standard library only, in the module and in
  every test. `go.mod` must never gain a `require` line.
- **No `float64` in parsing or arithmetic.** The only permitted use is inside
  `Float64()`, which is documented as display-only and must stay that way.
- **No silent rounding or truncation on any path.** An operation either returns
  the exact result or an error. `Round` is the single, explicitly named
  exception, and it is half-to-even.
- **No generics, no exported interfaces.** See the note at the end of this file.
- **No hard-coded business scale limits.** `MaxScale` is a property of the
  representation. A caller that accepts six decimal places enforces that at its
  own boundary.
- **Nothing panics except `MustParse`.** Internal unreachable states panic with a
  `"decimal: internal error: ..."` message, which is a programming error and
  never reachable from public input.
- **Signed overflow is checked by hand.** Go defines it as wrapping, so every
  multiplication, alignment and addition verifies its bound before or after the
  operation. Never assume a bound "cannot happen".
- **Everything in this repository is English**: code comments, godoc, the README,
  `AGENTS.md`, and commit messages. This is a public project, so English is not
  negotiable, whatever language defaults you may use elsewhere.

## Zero-allocation contract

These paths must stay at 0 allocs/op, and `alloc_test.go` fails the build if they
do not: `Cmp`, `Equal`, `LessThan`, `GreaterThan`, `Add`, `Sub`, `MulInt`, `Mul`,
`Round`, `Rescale`, `Parse`, `Scan` from a string, and every value query.

`String` is pinned at exactly 1 alloc (the returned string) and `MarshalJSON`,
`MarshalText`, `UnmarshalJSON` and `UnmarshalText` at exactly 1 (the returned
slice, or the `[]byte`-to-`string` conversion their signature forces). Do not
"fix" these by returning a pooled buffer: the values are immutable and callers
own the results.

Practical consequences when editing:

- Keep the text encoder writing into a stack buffer (`encodeDecimal`), never
  through `fmt`.
- Do not add `fmt` calls, `errors.New` calls or map lookups to a hot path. Error
  messages are built only on the failure path.
- Run the allocation tests after any change to a hot path, not just the
  benchmarks; the tests are the contract and the benchmarks are the evidence.

## Testing requirements

Every change must keep the full acceptance command green:

```sh
gofmt -l . && go build ./... && go vet ./... && go test ./... -race && go test -bench=. -benchmem ./...
```

When adding behaviour, add the test that would catch it being wrong:

- **Boundary cases** belong in the table-driven tests: `math.MaxInt64` and its
  neighbours, scale 0 and 18, `±0`, `".5"`, `"5."`, `"1e3"`, trailing zeros,
  19-digit literals, over-long fractions, malformed input.
- **Arithmetic and comparison** belong in `oracle_test.go` as differential tests
  against `math/big.Rat`, which is arbitrary precision and therefore exact. Add
  the new operation to `checkAgainstOracle` rather than writing a bespoke check.
- **Anything that parses or formats** belongs in a fuzz target. Fuzz targets must
  assert the invariants (`checkInvariants`), never panic, and be idempotent under
  round trips.
- **Examples are tests.** If you document a behaviour in `doc.go` or the README,
  consider pinning it with an `Example` function; they run in CI.
- Never weaken or delete a test to make a change pass. If a test is wrong,
  explain why in the change and fix the test's expectation explicitly.

Run fuzz targets locally with, for example:

```sh
go test -run '^FuzzCmpArith$' -fuzz '^FuzzCmpArith$' -fuzztime 30s .
```

## Commit messages

**Every commit in this repository follows the gitmoji format, stated in
English — without exception:**

```
<gitmoji> <one-sentence description>
```

- **The emoji is the type.** There is no `type(scope):` prefix and no scope, so
  no `feat:`, `fix:`, `docs:` and so on.
- **The description is one complete sentence in English**, not a `topic: detail`
  pair, and it does not end with a period.
- **Pick the single most representative emoji** for the change.

This repository's history is public and entirely in English, so commit messages
are English here too. That is this project's convention and it governs every
commit, whatever formatting or language defaults you may use elsewhere.

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

Examples:

```
✨ Implement half-to-even rounding in Decimal.Round
🐛 Fix rounding direction for negative coefficients
✅ Add big.Rat differential tests for cross-scale comparison
📝 Document the contributor conventions in AGENTS.md and README
⚡ Remove a redundant branch from the parse hot path
```

Commit often, and keep each message specific to one concern: if a change spans
unrelated concerns, split it into several commits rather than writing one vague
message. Commits and pushes are human decisions, so get approval for each one
rather than assuming a previous approval still stands, and never push,
force-push, retag or otherwise change a remote without being asked to.

## Why the hard rules exist

**On generics.** This package is deliberately concrete, for reasons that would
survive a request to make it generic. Go generics cannot express operator
constraints, so an integer-backed type parameter would need a method-based
constraint, which erases inlining on the hot path and adds a dispatch layer to
every operation. The type's whole design depends on the coefficient *being* an
`int64`: `10^-scale` and `|coef| <= MaxInt64` have no meaning for an arbitrary
type parameter. The only genuinely generic-looking surface is the database `Scan`
method (`src any`), and that signature is fixed by `database/sql`, not by this
package.

**On the strict `Scan`.** `Scan` accepts only `string`, `[]byte` and `nil`. It
deliberately refuses an `int64` or `float64` from a driver, because a decimal
that arrives as a `float64` has already lost its exact value, and accepting it
would hide that. Note the consequence, established by experiment: `database/sql`
checks its own fast paths for `string`, `[]byte`, `int64` and `float64` *before*
it calls `Scanner.Scan`, so this type only sees values whose column the driver
reports as text. A column with numeric affinity therefore fails loudly rather
than silently rounding. See the README's gorm section.
