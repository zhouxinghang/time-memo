package server

import "net/http"

func (a *Server) dailyNote(w http.ResponseWriter, r *http.Request) {
	note, err := a.notes.Get(r.Context())
	if err != nil {
		dbError(w, err)
		return
	}
	write(w, http.StatusOK, note)
}
