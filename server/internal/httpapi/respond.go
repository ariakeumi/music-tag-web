package httpapi

import (
	"encoding/json"
	"net/http"
)

// envelope replicates component/drf/viewsets.py success_response/failure_response:
// {"result": true, "code": "200", "data": [...], "message": "success"}
type envelope struct {
	Result  bool   `json:"result"`
	Code    string `json:"code"`
	Data    any    `json:"data"`
	Message string `json:"message"`
}

func Success(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, envelope{Result: true, Code: "200", Data: data, Message: "success"})
}

func SuccessMsg(w http.ResponseWriter, msg string, data any) {
	writeJSON(w, http.StatusOK, envelope{Result: true, Code: "200", Data: data, Message: msg})
}

func Failure(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusOK, envelope{Result: false, Code: "400", Data: []any{}, Message: msg})
}

func Unauthorized(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusUnauthorized, map[string]any{"detail": msg})
}

// PageData replicates CustomPageNumberPagination:
// {"page": 1, "total_page": 3, "count": 30, "items": [...]}
type PageData struct {
	Page      int   `json:"page"`
	TotalPage int   `json:"total_page"`
	Count     int   `json:"count"`
	Items     []any `json:"items"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func DecodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	return dec.Decode(v)
}
