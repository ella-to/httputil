package httputil

import (
	"errors"
	"fmt"
	"net/http"
	"time"
)

var (
	ErrParsingCookie = errors.New("parsing cookie")
)

type CookieKey string

// Set maxAge is in seconds, if maxAge is zero, it deletes the cookie
func SetCookie(w http.ResponseWriter, key CookieKey, value string, maxAge time.Duration, secure bool) {
	cookie := http.Cookie{
		Name:     string(key),
		Value:    value,
		Path:     "/",
		Secure:   secure,
		HttpOnly: true,
		MaxAge:   int(maxAge / time.Second),
		Expires:  time.Now().Add(maxAge),
	}

	http.SetCookie(w, &cookie)
}

// Get tries to extract cookie by name from request object
func GetCookie(key CookieKey, r *http.Request) (string, error) {
	cookie, err := r.Cookie(string(key))
	if err != nil {
		return "", fmt.Errorf("%w (%s): %w", ErrParsingCookie, key, err)
	}

	return cookie.Value, nil
}
