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

// IsForeignKeyViolation reports a write refused because a referenced row does
// not exist — typically a row that was deleted while a background job worked.
func IsForeignKeyViolation(err error) bool {
	if err == nil {
		return false
	}
	var serr *sqlite.Error
	if errors.As(err, &serr) && serr.Code() == sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY {
		return true
	}
	return strings.Contains(err.Error(), "FOREIGN KEY constraint failed")
}
