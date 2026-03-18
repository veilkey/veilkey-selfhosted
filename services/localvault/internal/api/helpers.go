package api

import "veilkey-localvault/internal/httputil"

func joinPath(base string, elem ...string) string { return httputil.JoinPath(base, elem...) }
