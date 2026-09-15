package decimal

import "database/sql/driver"

// Scan implements sql.Scanner. It accepts the text form of a decimal: a nil
// value (which yields the canonical zero), a string, or a []byte. Anything else
// fails, with a message naming the received type. The input goes through the
// same strict parser as Parse, so a database value that exceeds MaxScale or the
// coefficient range is reported as an error rather than silently truncated.
func (d *Decimal) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*d = Decimal{}
		return nil
	case string:
		dec, err := parseString(v)
		if err != nil {
			return err
		}
		*d = dec
		return nil
	case []byte:
		dec, err := parseString(string(v))
		if err != nil {
			return err
		}
		*d = dec
		return nil
	default:
		// A float64 or an int64 arriving from a driver is deliberately not
		// converted: a float64 has already lost the decimal value it stood for.
		return errScanType(src)
	}
}

// Value implements driver.Valuer. The Decimal is handed to the driver as its
// canonical string, so no precision is lost on the way to the database: every
// driver that accepts a string accepts this, and none of them can reinterpret
// the decimal as a binary float.
func (d Decimal) Value() (driver.Value, error) {
	return d.String(), nil
}
