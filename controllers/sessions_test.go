package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"deploycrate-ce/config"
	"deploycrate-ce/models"
	"deploycrate-ce/models/factories"
	"deploycrate-ce/queue"
	"deploycrate-ce/router/cookies"
	"deploycrate-ce/router/middleware"
	"deploycrate-ce/router/routes"
	"deploycrate-ce/services"

	"github.com/gorilla/sessions"
	"github.com/labstack/echo-contrib/v5/session"
	"github.com/labstack/echo/v5"
)

func TestSignIn(t *testing.T) {
	const (
		email    = "person@example.com"
		password = "correct horse battery staple"
		pepper   = "test-pepper"
	)

	cfg := signInTestConfig(t, pepper)
	tests := []struct {
		name              string
		createUser        bool
		verified          bool
		providedEmail     string
		providedPassword  string
		wantStatus        int
		wantLocation      string
		wantAuthenticated bool
		wantFlash         string
		wantErrors        map[string]string
		staleCookies      bool
	}{
		{
			name:              "verified user",
			createUser:        true,
			verified:          true,
			providedEmail:     email,
			providedPassword:  password,
			wantStatus:        http.StatusSeeOther,
			wantLocation:      routes.HomePage.URL(),
			wantAuthenticated: true,
			wantFlash:         "Successfully logged in!",
		},
		{
			name:              "verified user with stale session cookies",
			createUser:        true,
			verified:          true,
			providedEmail:     email,
			providedPassword:  password,
			wantStatus:        http.StatusSeeOther,
			wantLocation:      routes.HomePage.URL(),
			wantAuthenticated: true,
			wantFlash:         "Successfully logged in!",
			staleCookies:      true,
		},
		{
			name:             "wrong password",
			createUser:       true,
			verified:         true,
			providedEmail:    email,
			providedPassword: "wrong password",
			wantStatus:       http.StatusSeeOther,
			wantLocation:     routes.SessionNew.URL(),
			wantFlash:        "Invalid email or password",
		},
		{
			name:             "unverified user",
			createUser:       true,
			providedEmail:    email,
			providedPassword: password,
			wantStatus:       http.StatusSeeOther,
			wantLocation:     routes.SessionNew.URL(),
			wantFlash:        "Please verify your email before logging in",
		},
		{
			name:       "missing credentials",
			wantStatus: http.StatusOK,
			wantErrors: map[string]string{
				"email":    "is required",
				"password": "is required",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := newControllerTestDB(t, nil)
			if test.createUser {
				hash, err := models.HashPassword(password, pepper)
				if err != nil {
					t.Fatalf("hash password: %v", err)
				}
				options := []factories.UserOption{
					factories.WithEmail(email),
					factories.WithPassword([]byte(hash)),
				}
				if test.verified {
					options = append(options, factories.WithValidatedEmail())
				}
				if _, err := factories.CreateUser(
					t.Context(),
					db.Executor(),
					options...); err != nil {
					t.Fatalf("create user: %v", err)
				}
			}

			store := sessions.NewCookieStore([]byte("01234567890123456789012345678901"))
			controller := NewSessions(services.NewIdentity(db, queue.InsertOnly{}, cfg))
			var requestCookies []*http.Cookie
			if test.staleCookies {
				requestCookies = staleSessionCookies(t)
			}
			response := submitSignIn(
				t,
				store,
				controller,
				test.providedEmail,
				test.providedPassword,
				requestCookies,
			)

			if response.Code != test.wantStatus {
				t.Fatalf(
					"status = %d, want %d; body: %s",
					response.Code,
					test.wantStatus,
					response.Body.String(),
				)
			}
			if location := response.Header().Get("Location"); location != test.wantLocation {
				t.Fatalf("Location = %q, want %q", location, test.wantLocation)
			}
			if authenticated := authenticatedFromResponse(
				t,
				store,
				response,
			); authenticated != test.wantAuthenticated {
				t.Fatalf("authenticated = %t, want %t", authenticated, test.wantAuthenticated)
			}

			if test.wantFlash != "" {
				flashes := flashesFromResponse(t, store, response)
				if len(flashes) != 1 || flashes[0].Message != test.wantFlash {
					t.Fatalf("flashes = %+v, want message %q", flashes, test.wantFlash)
				}
			}

			if test.wantErrors != nil {
				var page struct {
					Component string `json:"component"`
					Props     struct {
						Errors map[string]string `json:"errors"`
					} `json:"props"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
					t.Fatalf("decode Inertia response: %v", err)
				}
				if page.Component != "Auth/Login" {
					t.Fatalf("component = %q, want Auth/Login", page.Component)
				}
				for field, message := range test.wantErrors {
					if page.Props.Errors[field] != message {
						t.Fatalf(
							"error for %s = %q, want %q",
							field,
							page.Props.Errors[field],
							message,
						)
					}
				}
			}
		})
	}
}

func submitSignIn(
	t *testing.T,
	store *sessions.CookieStore,
	controller Sessions,
	email string,
	password string,
	requestCookies []*http.Cookie,
) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(map[string]string{"email": email, "password": password})
	if err != nil {
		t.Fatalf("encode sign-in request: %v", err)
	}
	request := httptest.NewRequest(
		http.MethodPost,
		routes.SessionCreate.URL(),
		strings.NewReader(string(body)),
	)
	request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	request.Header.Set("X-Inertia", "true")
	for _, cookie := range requestCookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	ctx := echo.New().NewContext(request, response)

	handler := middleware.ValidateSession(controller.Create)
	if err := session.Middleware(store)(handler)(ctx); err != nil {
		t.Fatalf("submit sign-in: %v", err)
	}
	return response
}

func staleSessionCookies(t *testing.T) []*http.Cookie {
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

func authenticatedFromResponse(
	t *testing.T,
	store *sessions.CookieStore,
	response *httptest.ResponseRecorder,
) bool {
	t.Helper()

	var authenticated bool
	runWithResponseCookies(t, store, response, func(ctx *echo.Context) error {
		authenticated = cookies.ExtractFromCookieApp(ctx).IsAuthenticated
		return ctx.NoContent(http.StatusNoContent)
	})
	return authenticated
}

func flashesFromResponse(
	t *testing.T,
	store *sessions.CookieStore,
	response *httptest.ResponseRecorder,
) []cookies.FlashMessage {
	t.Helper()

	var flashes []cookies.FlashMessage
	runWithResponseCookies(t, store, response, func(ctx *echo.Context) error {
		var err error
		flashes, err = cookies.ExtractFlashes(ctx)
		return err
	})
	return flashes
}

func runWithResponseCookies(
	t *testing.T,
	store *sessions.CookieStore,
	response *httptest.ResponseRecorder,
	handler echo.HandlerFunc,
) {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, cookie := range latestResponseCookies(response.Result().Cookies()) {
		request.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	ctx := echo.New().NewContext(request, recorder)
	if err := session.Middleware(store)(handler)(ctx); err != nil {
		t.Fatalf("read response session: %v", err)
	}
}

func latestResponseCookies(cookies []*http.Cookie) []*http.Cookie {
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

func signInTestConfig(t *testing.T, pepper string) config.Config {
	t.Helper()

	for key, value := range map[string]string{
		"SESSION_KEY":               "0123456789012345678901234567890123456789012345678901234567890123",
		"SESSION_ENCRYPTION_KEY":    "0123456789012345678901234567890123456789012345678901234567890123",
		"TOKEN_SIGNING_KEY":         "test-signing-key",
		"PEPPER":                    pepper,
		"CORS_ALLOWED_ORIGINS":      "http://localhost:8080",
		"CSRF_TRUSTED_ORIGINS":      "http://localhost:8080",
		"DB_PORT":                   "5432",
		"DB_HOST":                   "localhost",
		"DB_NAME":                   "test",
		"DB_USER":                   "test",
		"DB_PASSWORD":               "test",
		"DB_KIND":                   "postgres",
		"DB_SSL_MODE":               "disable",
		"AWS_SES_ACCESS_KEY_ID":     "test",
		"AWS_SES_SECRET_ACCESS_KEY": "test",
		"AWS_SES_CONFIGURATION_SET": "test",
	} {
		t.Setenv(key, value)
	}
	return config.NewConfig()
}
