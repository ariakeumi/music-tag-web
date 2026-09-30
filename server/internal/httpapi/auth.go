package httpapi

import (
	"net/http"

	"github.com/xhongc/music-tag-web/server/internal/store"
)

// POST /api/token/ — replicates rest_framework_jwt obtain_jwt_token.
func (s *Server) HandleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := DecodeJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"non_field_errors": []string{"Invalid request."},
		})
		return
	}
	u, err := s.Store.GetUser(body.Username)
	if err != nil || !store.VerifyPassword(u.Password, body.Password) {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"non_field_errors": []string{"Unable to log in with provided credentials."},
		})
		return
	}
	token, err := IssueToken(s.Config.JWTSecret, u.Username)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"detail": "sign error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token})
}

// GET /user/info/ — the SPA header checks the standard result envelope on
// this endpoint (res.result) to decide whether to show the login page, so
// the response is always wrapped. With login disabled it reports the
// implicit admin so the UI proceeds without a login page.
func (s *Server) HandleUserInfo(w http.ResponseWriter, r *http.Request) {
	username := CurrentUser(r)
	if !s.Config.LoginRequired {
		Success(w, map[string]any{"username": "admin", "role": "admin"})
		return
	}
	role := "other"
	if u, err := s.Store.GetUser(username); err == nil {
		role = u.Role()
	}
	Success(w, map[string]any{"username": username, "role": role})
}
