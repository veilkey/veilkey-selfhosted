package api

import "github.com/veilkey/veilkey-go-package/httputil"

func joinPath(base string, elem ...string) string { return httputil.JoinPath(base, elem...) }
