package api

import "veilkey-vaultcenter/internal/httputil"

func isValidResourceName(name string) bool { return httputil.IsValidResourceName(name) }
