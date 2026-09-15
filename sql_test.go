package decimal

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
)

// registerOnce guards the one-time driver registration that database/sql
// requires.
var registerOnce sync.Once

// TestScan covers the accepted database sources: the text form, a byte slice,
// and NULL.
func TestScan(t *testing.T) {
	tests := []struct {
		name string
		src  any
		want string
	}{
		{"string", "1.50", "1.5"},
		{"string integer", "42", "42"},
		{"string negative", "-0.25", "-0.25"},
		{"string exponent", "1e-18", "0.000000000000000001"},
		{"bytes", []byte("1.50"), "1.5"},
		{"bytes max", []byte("9223372036854775807"), "9223372036854775807"},
		{"nil", nil, "0"},
		{"empty bytes", []byte{}, ""}, // invalid, checked separately below
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var d Decimal
			err := d.Scan(tc.src)
			if tc.name == "empty bytes" {
				if !errors.Is(err, ErrSyntax) {
					t.Fatalf("Scan([]byte{}) error = %v, want ErrSyntax", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Scan(%v) error = %v", tc.src, err)
			}
			if got := d.String(); got != tc.want {
				t.Fatalf("Scan(%v) = %s, want %s", tc.src, got, tc.want)
			}
			requireCanonical(t, d, "Scan")
		})
	}
}

// TestScanResetsToZero pins that scanning NULL yields the canonical zero even
// when the receiver held another value.
func TestScanResetsToZero(t *testing.T) {
	d := MustParse("12.5")
	if err := d.Scan(nil); err != nil {
		t.Fatalf("Scan(nil) error = %v", err)
	}
	if d != (Decimal{}) {
		t.Fatalf("Scan(nil) = %v, want the canonical zero", d)
	}
}

// TestScanErrors covers the sources the type refuses, including the numeric ones
// that would already have lost their decimal value.
func TestScanErrors(t *testing.T) {
	sources := []any{
		1.5, float64(1), int64(1), int32(1), true, []int{1},
		[]byte("oops"), "oops", "", "1e-19", "9999999999999999999",
	}
	for _, src := range sources {
		var d Decimal
		if err := d.Scan(src); err == nil {
			t.Errorf("Scan(%#v) = %v, want an error", src, d)
		}
	}
}

// TestValue covers driver.Valuer: the canonical string, and the fact that it
// satisfies the interface the database packages look for.
func TestValue(t *testing.T) {
	var _ driver.Valuer = Decimal{}
	var _ sql.Scanner = &Decimal{}

	tests := []struct {
		in   string
		want string
	}{
		{"0", "0"},
		{"-0.0", "0"},
		{"1.50", "1.5"},
		{"1e3", "1000"},
		{"1e-18", "0.000000000000000001"},
		{"9223372036854775807", "9223372036854775807"},
	}
	for _, tc := range tests {
		got, err := MustParse(tc.in).Value()
		if err != nil {
			t.Errorf("%s.Value() error = %v", tc.in, err)
			continue
		}
		s, ok := got.(string)
		if !ok {
			t.Fatalf("%s.Value() = %#v, want a string", tc.in, got)
		}
		if s != tc.want {
			t.Errorf("%s.Value() = %q, want %q", tc.in, s, tc.want)
		}
	}
}

// TestScanValueRoundTrip checks that the database representation survives a
// round trip, which is what makes the text based Value safe.
func TestScanValueRoundTrip(t *testing.T) {
	literals := []string{
		"0", "1", "-1", "1.5", "-1.5", "1e-18", "9223372036854775807",
		"-9223372036854775807", "1.000000000000000001", "1000.001",
	}
	for _, s := range literals {
		d := MustParse(s)
		v, err := d.Value()
		if err != nil {
			t.Fatalf("%s.Value(): %v", s, err)
		}
		var back Decimal
		if err := back.Scan(v); err != nil {
			t.Fatalf("Scan(%v): %v", v, err)
		}
		if back != d {
			t.Fatalf("round trip of %s gave %s", s, back)
		}
	}
}

// TestScanReturnsPartialResultsNotGarbage pins that a failed Scan does not leave
// a half written receiver behind.
func TestScanReturnsPartialResultsNotGarbage(t *testing.T) {
	d := MustParse("7.5")
	if err := d.Scan("not a number"); err == nil {
		t.Fatal("Scan of junk must fail")
	}
	if d != MustParse("7.5") {
		t.Fatalf("a failed Scan changed the receiver to %v", d)
	}
}

// TestScanThroughDatabaseSQL checks the Scanner against the real database/sql
// path rather than a direct method call. The distinction matters: database/sql
// inspects the error a Scanner returns and replaces it with its own
// ErrScanUnsupported, unless the error unwraps to driver.ErrSkip. Scanning a
// kind this type refuses surfaces ErrScanUnsupported to the caller, while a
// value that is text but not a decimal surfaces the parser's own sentinel.
func TestScanThroughDatabaseSQL(t *testing.T) {
	registerTextDriver(t)

	db, err := sql.Open("decimaltext", "")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	t.Run("accepted", func(t *testing.T) {
		var d Decimal
		if err := db.QueryRow("select val").Scan(&d); err != nil {
			t.Fatalf("Scan: %v", err)
		}
		if d.String() != "1.5" {
			t.Fatalf("Scan = %v, want 1.5", d)
		}
	})

	t.Run("null becomes zero", func(t *testing.T) {
		d := MustParse("9")
		if err := db.QueryRow("select null").Scan(&d); err != nil {
			t.Fatalf("Scan: %v", err)
		}
		if !d.IsZero() {
			t.Fatalf("Scan of NULL = %v, want zero", d)
		}
	})

	t.Run("bytes source", func(t *testing.T) {
		var d Decimal
		if err := db.QueryRow("select bytes").Scan(&d); err != nil {
			t.Fatalf("Scan: %v", err)
		}
		if d.String() != "-0.25" {
			t.Fatalf("Scan of []byte = %v, want -0.25", d)
		}
	})

	t.Run("invalid text keeps the parser error", func(t *testing.T) {
		var d Decimal
		err := db.QueryRow("select junk").Scan(&d)
		if !errors.Is(err, ErrSyntax) {
			t.Fatalf("Scan of junk error = %v, want ErrSyntax", err)
		}
	})

	t.Run("out of range keeps the range error", func(t *testing.T) {
		var d Decimal
		err := db.QueryRow("select toosmall").Scan(&d)
		if !errors.Is(err, ErrScaleOutOfRange) {
			t.Fatalf("Scan of 1e-19 error = %v, want ErrScaleOutOfRange", err)
		}
	})

	t.Run("unsupported kind is refused", func(t *testing.T) {
		var d Decimal
		err := db.QueryRow("select float").Scan(&d)
		if err == nil {
			t.Fatalf("Scan of a float64 succeeded with %v, want an error", d)
		}
		if !strings.Contains(err.Error(), "cannot scan") {
			t.Fatalf("Scan of a float64 error = %v, want a message naming the type", err)
		}
		if d != (Decimal{}) {
			t.Fatalf("Scan of a float64 left %v behind", d)
		}
	})
}

// decimalTextDriver is a minimal driver.Value-producing driver, used only to
// exercise the Scanner through database/sql.
type decimalTextDriver struct{}

func (decimalTextDriver) Open(string) (driver.Conn, error) { return decimalTextConn{}, nil }

type decimalTextConn struct{}

func (decimalTextConn) Prepare(query string) (driver.Stmt, error) {
	return decimalTextStmt{query: query}, nil
}
func (decimalTextConn) Close() error              { return nil }
func (decimalTextConn) Begin() (driver.Tx, error) { return nil, errors.New("read only") }

type decimalTextStmt struct{ query string }

func (s decimalTextStmt) Close() error  { return nil }
func (s decimalTextStmt) NumInput() int { return 0 }
func (s decimalTextStmt) Exec([]driver.Value) (driver.Result, error) {
	return nil, errors.New("read only")
}

func (s decimalTextStmt) Query([]driver.Value) (driver.Rows, error) {
	var value driver.Value
	switch s.query {
	case "select val":
		value = "1.50"
	case "select null":
		value = nil
	case "select bytes":
		value = []byte("-0.25")
	case "select junk":
		value = "not a decimal"
	case "select toosmall":
		value = "1e-19"
	case "select float":
		value = 1.5
	default:
		return nil, fmt.Errorf("decimaltext: unknown query %q", s.query)
	}
	return &decimalTextRows{value: value}, nil
}

type decimalTextRows struct {
	value driver.Value
	done  bool
}

func (r *decimalTextRows) Columns() []string { return []string{"val"} }
func (r *decimalTextRows) Close() error      { return nil }
func (r *decimalTextRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	dest[0] = r.value
	return nil
}

// registerTextDriver installs the driver once per process, since database/sql
// panics on a duplicate registration.
func registerTextDriver(t *testing.T) {
	t.Helper()
	registerOnce.Do(func() {
		sql.Register("decimaltext", decimalTextDriver{})
	})
}
