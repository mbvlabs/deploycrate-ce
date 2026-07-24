package cookies

import (
	"strings"

	"deploycrate-ce/config"
	"deploycrate-ce/models"
	"github.com/google/uuid"

	"github.com/labstack/echo/v5"
)

const ReturnToKey = "return_to"

const (
	isAuthenticated = "is_authenticated"
	isAdmin         = "is_admin"
	userID          = "user_id"
	email           = "email"
)

type App struct {
	CurrentPath     string
	UserID          uuid.UUID
	Email           string
	IsAdmin         bool
	IsAuthenticated bool
}

func CreateAppSession(c *echo.Context, user models.UserEntity) error {
	sess, err := getSession(config.AppCookieSessionName, c)
	if err != nil {
		return err
	}

	sess.Values[isAuthenticated] = true
	sess.Values[isAdmin] = user.IsAdmin
	sess.Values[userID] = user.ID.String()
	sess.Values[email] = user.Email

	return sess.Save(c.Request(), c.Response())
}

func DestroyAppSession(c *echo.Context) error {
	sess, err := getSession(config.AppCookieSessionName, c)
	if err != nil {
		return err
	}

	sess.Options.MaxAge = -1
	return sess.Save(c.Request(), c.Response())
}

func ExtractFromCookieApp(c *echo.Context) App {
	sess, err := getSession(config.AppCookieSessionName, c)
	if err != nil {
		return App{}
	}

	app := App{}

	if v, ok := sess.Values[isAuthenticated].(bool); ok {
		app.IsAuthenticated = v
	}
	if v, ok := sess.Values[isAdmin].(bool); ok {
		app.IsAdmin = v
	}
	if v, ok := sess.Values[userID].(string); ok {
		app.UserID, _ = uuid.Parse(v)
	}
	if v, ok := sess.Values[email].(string); ok {
		app.Email = v
	}

	return app
}

func SetReturnTo(c *echo.Context, returnTo string) error {
	sess, err := getSession(config.AppCookieSessionName, c)
	if err != nil {
		return err
	}

	if returnTo == "" {
		delete(sess.Values, ReturnToKey)
	} else {
		sess.Values[ReturnToKey] = returnTo
	}

	return sess.Save(c.Request(), c.Response())
}

func GetReturnTo(c *echo.Context) string {
	sess, err := getSession(config.AppCookieSessionName, c)
	if err != nil {
		return ""
	}

	returnTo, _ := sess.Values[ReturnToKey].(string)
	return strings.TrimSpace(returnTo)
}

func ResolveReturnTo(c *echo.Context, fallback string) string {
	returnTo := GetReturnTo(c)
	if !isSafeReturnTo(returnTo) {
		return fallback
	}

	return returnTo
}

func isSafeReturnTo(returnTo string) bool {
	if returnTo == "" {
		return false
	}

	return strings.HasPrefix(returnTo, "/") &&
		!strings.HasPrefix(returnTo, "//")
}
