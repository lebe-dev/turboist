package repo

import "strings"

func joinSets(sets []string) string {
	return strings.Join(sets, ", ")
}
