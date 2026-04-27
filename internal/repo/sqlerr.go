package repo

import (
	"errors"
	"strings"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

func isUniqueViolation(err error) bool {
	var serr *sqlite.Error
	if errors.As(err, &serr) {
		code := serr.Code()
		if code == sqlite3.SQLITE_CONSTRAINT_UNIQUE || code == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY {
			return true
		}
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
