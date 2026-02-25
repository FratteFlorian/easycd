package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMiddleware(t *testing.T) {
	const secret = "my-secret-token"

	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := Middleware(secret, ok)

	cases := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{"valid token", "Bearer my-secret-token", http.StatusOK},
		{"missing header", "", http.StatusUnauthorized},
		{"wrong token", "Bearer wrong-token", http.StatusUnauthorized},
		{"no bearer prefix", "my-secret-token", http.StatusUnauthorized},
		{"wrong scheme", "Basic my-secret-token", http.StatusUnauthorized},
		{"bearer case insensitive", "bearer my-secret-token", http.StatusOK},
		{"BEARER uppercase", "BEARER my-secret-token", http.StatusOK},
		{"empty token value", "Bearer ", http.StatusUnauthorized},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if c.authHeader != "" {
				req.Header.Set("Authorization", c.authHeader)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != c.wantStatus {
				t.Errorf("got status %d, want %d", w.Code, c.wantStatus)
			}
		})
	}
}
