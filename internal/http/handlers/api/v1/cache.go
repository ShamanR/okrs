package v1

import "net/http"

// setAPICacheControl marks tenant-scoped API responses non-shared and always-revalidated.
// They must NOT be `public`-cached: a cached response is reused by the browser after a tenant
// switch (stale sidebar/hierarchy) and could leak one tenant's data to another via a shared
// cache. `private, no-cache` lets the browser store but forces revalidation, so each tenant
// gets a fresh response. (Design spec §"Кэши и изоляция (HTTP)", Вариант B; ETag — later opt.)
func setAPICacheControl(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "private, no-cache")
}

func SetAPICacheControl(w http.ResponseWriter) {
	setAPICacheControl(w)
}
