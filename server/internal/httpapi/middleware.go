package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

const JWTExpiration = 7 * 24 * time.Hour

type ctxKey string

const userKey ctxKey = "username"

// IssueToken signs a drf-jwt-compatible payload (HS256, 7 day expiry).
func IssueToken(secret, username string) (string, error) {
	claims := jwt.MapClaims{
		"user_id":  0,
		"username": username,
		"jti":      username,
		"exp":      time.Now().Add(JWTExpiration).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func CurrentUser(r *http.Request) string {
	if v, ok := r.Context().Value(userKey).(string); ok {
		return v
	}
	return ""
}

// AuthMiddleware validates "Authorization: JWT/jwt <token>" — the frontend
// copies this value from the AUTHORIZATION cookie it set at login. When
// LoginRequired is off, requests pass through unauthenticated.
func AuthMiddleware(secret string, required bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !required {
			next.ServeHTTP(w, r)
			return
		}
		header := r.Header.Get("Authorization")
		if header == "" {
			header = r.Header.Get("AUTHORIZATION")
		}
		if header == "" {
			// fall back to the cookie the frontend sets at login
			if c, err := r.Cookie("AUTHORIZATION"); err == nil {
				header = c.Value
			}
		}
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "JWT") || parts[1] == "" {
			Unauthorized(w, "Error decoding signature.")
			return
		}
		token, err := jwt.Parse(parts[1], func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(secret), nil
		})
		if err != nil || !token.Valid {
			Unauthorized(w, "Signature has expired.")
			return
		}
		claims := token.Claims.(jwt.MapClaims)
		username, _ := claims["username"].(string)
		if username == "" {
			if _, ok := claims["exp"]; !ok {
				Unauthorized(w, "Invalid token.")
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, username)))
	})
}

// GzipMiddleware compresses JSON responses, mirroring Django's gzip_page.
func GzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Del("Content-Length")
		gz := newGzipWriter(w)
		defer gz.Close()
		next.ServeHTTP(gz, r)
	})
}
