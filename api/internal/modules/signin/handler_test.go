package signin

import "testing"

func TestSafeReturnStaysInTheApp(t *testing.T) {
	for in, want := range map[string]string{
		"/servers": "/servers", "/discovery?kind=needs": "/discovery?kind=needs", "": "/", "https://evil.example": "/",
		"//evil.example": "/", "/\\evil.example": "/", "/api/v1/auth/logout": "/", "servers": "/",
	} {
		if got := safeReturn(in); got != want {
			t.Errorf("safeReturn(%q) = %q, want %q", in, got, want)
		}
	}
}
