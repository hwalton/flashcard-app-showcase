package utils

import (
	"net/http"
	"os"
	"strings"
	"time"
)

// SetCookie sets a cookie with consistent defaults (HttpOnly, SameSite=Lax, Secure per IsSecureRequest).
// If expires.IsZero() the cookie will be a session cookie (no Expires/MaxAge header).
func SetCookie(w http.ResponseWriter, r *http.Request, name, value string, expires time.Time) {
	secure := IsSecureRequest(r)
	c := &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
	if !expires.IsZero() {
		c.Expires = expires
		// set MaxAge for compatibility; compute seconds until expiry
		c.MaxAge = int(time.Until(expires).Round(time.Second).Seconds())
	}
	http.SetCookie(w, c)
}

// ClearCookie removes a cookie using the same security flags.
func ClearCookie(w http.ResponseWriter, r *http.Request, name string) {
	c := &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   IsSecureRequest(r),
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, c)
}

func IsSecureRequest(r *http.Request) bool {
	if dev := os.Getenv("DEVELOPMENT_MODE"); dev != "" {
		if strings.EqualFold(dev, "1") || strings.EqualFold(dev, "true") || strings.EqualFold(dev, "yes") {
			return false
		}
	}
	return true
}
