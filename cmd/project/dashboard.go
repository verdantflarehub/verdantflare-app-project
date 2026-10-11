package main

import (
	"net/http"
	"strings"
)

// Dashboard routes retain the internal service's authentication, strict request
// decoder and existing result/error envelopes. They do not expose general commit.
func registerDashboardAPI(mux *http.ServeMux, internal http.Handler) {
	for _, operation := range []string{"list", "open", "create", "use_asset", "commit_status", "publication_prepare", "publication_receipt", "publication_associate"} {
		maximum := int64(1 << 20)
		if strings.HasPrefix(operation, "publication_") {
			maximum = 16 << 10
		}
		mux.HandleFunc("/api/v1/project/"+operation, func(w http.ResponseWriter, r *http.Request) {
			copy := r.Clone(r.Context())
			copy.URL.Path = "/internal/v1/project/" + operation
			copy.Body = http.MaxBytesReader(w, r.Body, maximum)
			internal.ServeHTTP(w, copy)
		})
	}
}
