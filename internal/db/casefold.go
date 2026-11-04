package db

import (
	"database/sql/driver"
	"strings"

	"modernc.org/sqlite"
)

// CasefoldFunc is the name of the SQL function that lower-cases text with Go's
// Unicode tables. SQLite's own LIKE and lower() only fold ASCII, so a search for
// "отп" would never find "Отпуск"; comparing casefold(column) against a pattern
// lower-cased in Go makes the match case-insensitive for every script.
const CasefoldFunc = "casefold"

func init() {
	sqlite.MustRegisterDeterministicScalarFunction(CasefoldFunc, 1, casefold)
}

func casefold(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	switch v := args[0].(type) {
	case nil:
		return nil, nil
	case string:
		return Casefold(v), nil
	case []byte:
		return Casefold(string(v)), nil
	default:
		return v, nil
	}
}

// Casefold is the Go side of the casefold SQL function: patterns compared
// against casefold(column) must be folded the same way.
func Casefold(s string) string {
	return strings.ToLower(s)
}
