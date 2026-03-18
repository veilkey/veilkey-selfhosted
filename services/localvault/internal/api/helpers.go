package api

import "net/url"

// joinPath joins a base URL with path elements. Panics if base is not a valid URL,
// which would always indicate a programming error with a hard-coded base.
func joinPath(base string, elem ...string) string {
	result, err := url.JoinPath(base, elem...)
	if err != nil {
		panic("api.joinPath: " + err.Error())
	}
	return result
}
