package api

import "net/http"

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	report, err := s.sched.Report()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"report": report, "summary": report.Summary()})
}
