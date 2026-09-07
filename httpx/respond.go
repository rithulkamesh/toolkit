package httpx

import (
	"encoding/json"
	"net/http"
)

// Error writes a JSON error response: {"error":"<msg>","code":"<code>"}.
// code may be empty.
func Error(w http.ResponseWriter, status int, msg, code string) {
	JSON(w, status, map[string]string{"error": msg, "code": code})
}

// JSON writes v as an indented JSON body with the given status code.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}
