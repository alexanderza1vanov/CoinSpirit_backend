package handler

import "net/http"

func RegisterHealthRoutes(router *http.ServeMux) {
	router.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
}
