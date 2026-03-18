package httputil

import "net/url"

// JoinPath joins a base URL with path elements. Panics if base is not a valid URL,
// which would always indicate a programming error with a hard-coded base.
func JoinPath(base string, elem ...string) string {
	result, err := url.JoinPath(base, elem...)
	if err != nil {
		panic("httputil.JoinPath: " + err.Error())
	}
	return result
}
