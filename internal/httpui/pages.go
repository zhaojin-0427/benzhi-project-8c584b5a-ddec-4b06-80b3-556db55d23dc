package httpui

import (
	"io"
	"net/http"
)

func (s *Server) Workbench(w http.ResponseWriter, r *http.Request) {
	file, err := assets.Open("assets/index.html")
	if err != nil {
		writeError(w, err)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, file)
}
