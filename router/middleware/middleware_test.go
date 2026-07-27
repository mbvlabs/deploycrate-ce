package middleware

import (
	"encoding/gob"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"deploycrate-ce/config"
	"deploycrate-ce/router/cookies"

	"github.com/gorilla/sessions"
	"github.com/labstack/echo-contrib/v5/session"
	"github.com/labstack/echo/v5"
)

func TestValidateSessionRecoversStaleCookies(t *testing.T) {
	gob.Register(cookies.FlashMessage{})
	staleCookies := createStaleCookies(t)
	currentStore := sessions.NewCookieStore([]byte("01234567890123456789012345678901"))

	request := httptest.NewRequest(http.MethodGet, "/login", nil)
	for _, cookie := range staleCookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	ctx := echo.New().NewContext(request, response)
	handler := ValidateSession(RegisterRequestMeta(func(c *echo.Context) error {
		if cookies.ExtractFromCookieApp(c).IsAuthenticated {
			t.Fatal("stale application session was trusted")
		}
		return c.NoContent(http.StatusNoContent)
	}))

	if err := session.Middleware(currentStore)(handler)(ctx); err != nil {
		t.Fatalf("handle stale sessions: %v", err)
	}
	replacementCookies := latestCookiesByName(response.Result().Cookies())
	if len(replacementCookies) != 2 {
		t.Fatalf("replacement cookies = %d, want 2", len(replacementCookies))
	}

	followUp := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, cookie := range replacementCookies {
		followUp.AddCookie(cookie)
	}
	followUpResponse := httptest.NewRecorder()
	followUpContext := echo.New().NewContext(followUp, followUpResponse)
	readSessions := func(c *echo.Context) error {
		if cookies.ExtractFromCookieApp(c).IsAuthenticated {
			t.Fatal("replacement application session is authenticated")
		}
		flashes, err := cookies.ExtractFlashes(c)
		if err != nil {
			return err
		}
		if len(flashes) != 0 {
			t.Fatalf("replacement flashes = %d, want 0", len(flashes))
		}
		return c.NoContent(http.StatusNoContent)
	}
	if err := session.Middleware(currentStore)(readSessions)(followUpContext); err != nil {
		t.Fatalf("read replacement sessions: %v", err)
	}
}

func TestValidateSessionPreservesNonDecodeErrors(t *testing.T) {
	want := errors.New("session store unavailable")
	store := &failingSessionStore{err: want}
	request := httptest.NewRequest(http.MethodGet, "/login", nil)
	response := httptest.NewRecorder()
	ctx := echo.New().NewContext(request, response)
	called := false
	handler := ValidateSession(func(c *echo.Context) error {
		called = true
		return c.NoContent(http.StatusNoContent)
	})

	err := session.Middleware(store)(handler)(ctx)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
	if called {
		t.Fatal("handler ran after a non-decode session error")
	}
}

func createStaleCookies(t *testing.T) []*http.Cookie {
	t.Helper()

	store := sessions.NewCookieStore([]byte("abcdefghijklmnopqrstuvwxyzABCDEF"))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	ctx := echo.New().NewContext(request, response)
	handler := func(c *echo.Context) error {
		sess, err := session.Get(config.AppCookieSessionName, c)
		if err != nil {
			return err
		}
		sess.Values["stale"] = true
		if err := sess.Save(c.Request(), c.Response()); err != nil {
			return err
		}
		return cookies.AddFlash(c, cookies.FlashInfo, "stale")
	}
	if err := session.Middleware(store)(handler)(ctx); err != nil {
		t.Fatalf("create stale sessions: %v", err)
	}
	return response.Result().Cookies()
}

func latestCookiesByName(cookies []*http.Cookie) []*http.Cookie {
	latest := make(map[string]*http.Cookie, len(cookies))
	for _, cookie := range cookies {
		latest[cookie.Name] = cookie
	}

	result := make([]*http.Cookie, 0, len(latest))
	for _, cookie := range latest {
		result = append(result, cookie)
	}
	return result
}

type failingSessionStore struct {
	err error
}

func (s *failingSessionStore) Get(request *http.Request, name string) (*sessions.Session, error) {
	return sessions.NewSession(s, name), s.err
}

func (s *failingSessionStore) New(request *http.Request, name string) (*sessions.Session, error) {
	return sessions.NewSession(s, name), s.err
}

func (s *failingSessionStore) Save(
	request *http.Request,
	response http.ResponseWriter,
	session *sessions.Session,
) error {
	return nil
}

func TestAPIPathBoundary(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "exact prefix", path: "/api", want: true},
		{name: "below prefix", path: "/api/users", want: true},
		{name: "prefix contained in segment", path: "/v1/api/users", want: false},
		{name: "prefix starts another segment", path: "/apiary", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isAPIPath(test.path); got != test.want {
				t.Fatalf("isAPIPath(%q) = %t, want %t", test.path, got, test.want)
			}
		})
	}
}

func TestCSRFBypassRequiresBearerWithoutApplicationSession(t *testing.T) {
	tests := []struct {
		name          string
		authorization string
		path          string
		sessionCookie bool
		want          bool
	}{
		{name: "bearer API request", authorization: "Bearer token", path: "/api/users", want: true},
		{name: "empty authorization", path: "/api/users", want: false},
		{name: "empty bearer token", authorization: "Bearer", path: "/api/users", want: false},
		{
			name:          "malformed bearer header",
			authorization: "Bearer token extra",
			path:          "/api/users",
			want:          false,
		},
		{
			name:          "other authorization scheme",
			authorization: "Basic token",
			path:          "/api/users",
			want:          false,
		},
		{name: "non API path", authorization: "Bearer token", path: "/users", want: false},
		{
			name:          "cookie authenticated API request",
			authorization: "Bearer token",
			path:          "/api/users",
			sessionCookie: true,
			want:          false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, test.path, nil)
			request.Header.Set("Authorization", test.authorization)
			if test.sessionCookie {
				request.AddCookie(&http.Cookie{Name: config.AppCookieSessionName, Value: "session"})
			}

			if got := mayBypassCSRF(request); got != test.want {
				t.Fatalf("mayBypassCSRF() = %t, want %t", got, test.want)
			}
		})
	}
}
