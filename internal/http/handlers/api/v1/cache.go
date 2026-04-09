package v1

import (
	"fmt"
	"net/http"
)

const apiCacheMaxAgeSeconds = 300

func setAPICacheControl(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", apiCacheMaxAgeSeconds))
}

func SetAPICacheControl(w http.ResponseWriter) {
	setAPICacheControl(w)
}
