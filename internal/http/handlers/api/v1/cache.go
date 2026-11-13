package v1

import "net/http"

// setAPICacheControl marks tenant-scoped API responses non-shared and always-revalidated.
// They must NOT be `public`-cached: a cached response is reused by the browser after a tenant
// switch (stale sidebar/hierarchy) and could leak one tenant's data to another via a shared
// cache. `private, no-cache` lets the browser store but forces revalidation, so each tenant
// gets a fresh response. (Design spec §"Кэши и изоляция (HTTP)", Вариант B; ETag — later opt.)
// `Vary: X-Tenant-ID` makes the stored entry correct by construction: the tenant is part of
// the request, so two spaces never share one cached response even heuristically.
func setAPICacheControl(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Add("Vary", "X-Tenant-ID")
}

func SetAPICacheControl(w http.ResponseWriter) {
	setAPICacheControl(w)
}
