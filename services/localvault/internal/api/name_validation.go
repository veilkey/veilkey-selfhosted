package api

import "regexp"

var validResourceName = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

func isValidResourceName(name string) bool {
	return validResourceName.MatchString(name)
}

const maxBulkItems = 200
