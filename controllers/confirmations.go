package controllers

import (
	"errors"
	"log/slog"
	"net/http"

	"deploycrate-ce/internal/inertia"
	"deploycrate-ce/internal/validation"
	"deploycrate-ce/router"
	"deploycrate-ce/router/cookies"
	"deploycrate-ce/router/routes"
	"deploycrate-ce/services"

	"github.com/labstack/echo/v5"
)

type Confirmations struct {
	identity services.Identity
}

func NewConfirmations(identity services.Identity) Confirmations {
	return Confirmations{identity}
}

func (c Confirmations) RegisterRoutes(r *router.Router) error {
	return registerRoutes(r, []routeDefinition{
		{http.MethodGet, routes.ConfirmationNew, c.New},
		{http.MethodPost, routes.ConfirmationCreate, c.Create},
	})
}

func (c Confirmations) New(etx *echo.Context) error {
	return inertia.Page(etx, "Auth/ConfirmEmail", inertia.Props{})
}

func (c Confirmations) Create(etx *echo.Context) error {
	var payload struct {
		Code string `json:"code"`
	}

	if err := etx.Bind(&payload); err != nil {
		slog.ErrorContext(
			etx.Request().Context(),
			"could not parse verification form payload",
			"error",
			err,
		)
		return inertia.Page(etx, "Errors/BadRequest", inertia.Props{})
	}

	user, err := c.identity.VerifyEmail(
		etx.Request().Context(),
		services.VerifyEmailData{
			Code: payload.Code,
		},
	)
	if err != nil {
		if validationErrors, ok := validation.As(err); ok {
			return inertia.Page(
				etx,
				"Auth/ConfirmEmail",
				inertia.Props{},
				inertia.WithValidationErrors(validationErrors.ToMap()),
			)
		}

		slog.ErrorContext(
			etx.Request().Context(),
			"failed to verify email",
			"error",
			err,
		)

		var errorMsg string
		switch {
		case errors.Is(err, services.ErrInvalidVerificationCode):
			errorMsg = "Invalid verification code"
		case errors.Is(err, services.ErrExpiredVerificationCode):
			errorMsg = "Verification code has expired"
		default:
			errorMsg = "Failed to verify email"
		}

		if flashErr := cookies.AddFlash(etx, cookies.FlashError, errorMsg); flashErr != nil {
			return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
		}
		return inertia.Redirect(etx, routes.ConfirmationNew.URL())
	}

	if err := cookies.CreateAppSession(etx, user); err != nil {
		slog.ErrorContext(
			etx.Request().Context(),
			"failed to create session",
			"error",
			err,
		)

		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}

	if flashErr := cookies.AddFlash(
		etx,
		cookies.FlashSuccess,
		"Email verified successfully!",
	); flashErr != nil {
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}

	return inertia.Location(etx, routes.HomePage.URL())
}
