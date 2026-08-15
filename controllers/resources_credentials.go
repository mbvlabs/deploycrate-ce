package controllers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"deploycrate-ce/models"
	"deploycrate-ce/router/cookies"
	"deploycrate-ce/services"

	"github.com/labstack/echo/v5"
)

type resourceCredentialPayload struct {
	Name         string            `json:"name"`
	Username     string            `json:"username"`
	Metadata     json.RawMessage   `json:"metadata"`
	SecretValues map[string]string `json:"secretValues"`
	Rotate       bool              `json:"rotate"`
}

func (payload resourceCredentialPayload) serviceInput() (services.ResourceCredentialInput, error) {
	return services.ResourceCredentialInput{
		Name: payload.Name, Username: payload.Username, Metadata: payload.Metadata,
		SecretValues: payload.SecretValues,
	}, nil
}

func (controller Resources) CreateCredential(etx *echo.Context) error {
	resourceID, err := uuidPathParam(etx, "id")
	var payload resourceCredentialPayload
	if err == nil {
		err = etx.Bind(&payload)
	}
	var input services.ResourceCredentialInput
	if err == nil {
		input, err = payload.serviceInput()
	}
	if err == nil {
		_, err = controller.service.CreateCredential(etx.Request().Context(), resourceID, input)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Credential created")
}

type resourceDatabasePayload struct {
	Name         string                     `json:"name"`
	Encoding     string                     `json:"encoding"`
	Collation    string                     `json:"collation"`
	CredentialID string                     `json:"credentialId"`
	Credential   *resourceCredentialPayload `json:"credential"`
}

func (controller Resources) CreateDatabase(etx *echo.Context) error {
	resourceID, err := uuidPathParam(etx, "id")
	var payload resourceDatabasePayload
	if err == nil {
		err = etx.Bind(&payload)
	}
	if err == nil {
		credentialID, credentialIDErr := optionalUUID(payload.CredentialID)
		if credentialIDErr != nil {
			err = domainPayloadError("credentialId", "credential is invalid")
		} else {
			input := services.CreateResourceDatabaseInput{
				Database: models.ResourceDatabaseDefinition{
					Name: payload.Name, Encoding: payload.Encoding, Collation: payload.Collation,
				},
				CredentialID: credentialID,
			}
			if payload.Credential != nil {
				var credential services.ResourceCredentialInput
				credential, err = payload.Credential.serviceInput()
				if err == nil {
					input.Credential = &credential
				}
			}
			if err == nil {
				_, err = controller.service.CreateDatabase(
					etx.Request().Context(),
					resourceID,
					input,
				)
			}
		}
	}
	return controller.finishChildMutation(
		etx,
		resourceID,
		err,
		"Database and credential access created",
	)
}

func (controller Resources) DestroyDatabase(etx *echo.Context) error {
	resourceID, err := uuidPathParam(etx, "id")
	databaseName := strings.TrimSpace(etx.Param("databaseName"))
	if err == nil && databaseName == "" {
		err = domainPayloadError("database", "Database is required")
	}
	if err == nil {
		err = controller.service.DestroyDatabase(
			etx.Request().Context(),
			resourceID,
			databaseName,
		)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Database deleted")
}

func (controller Resources) UpdateCredential(etx *echo.Context) error {
	resourceID, credentialID, err := parseChildIDs(etx, "credentialID")
	var payload resourceCredentialPayload
	if err == nil {
		err = etx.Bind(&payload)
	}
	var input services.ResourceCredentialInput
	if err == nil {
		input, err = payload.serviceInput()
	}
	if err == nil && payload.Rotate {
		_, err = controller.service.RotateCredential(
			etx.Request().Context(),
			resourceID,
			credentialID,
			input,
		)
	} else if err == nil {
		_, err = controller.service.UpdateCredentialMetadata(
			etx.Request().Context(),
			resourceID,
			credentialID,
			input,
		)
	}
	message := "Credential updated"
	if payload.Rotate {
		message = "Credential rotated"
	}
	return controller.finishChildMutation(etx, resourceID, err, message)
}

func (controller Resources) DestroyCredential(etx *echo.Context) error {
	resourceID, credentialID, err := parseChildIDs(etx, "credentialID")
	if err == nil {
		err = controller.service.ArchiveCredential(
			etx.Request().Context(),
			resourceID,
			credentialID,
		)
	}
	return controller.finishChildMutation(etx, resourceID, err, "Credential archived")
}

func (controller Resources) RevealCredential(etx *echo.Context) error {
	etx.Response().Header().Set("Cache-Control", "no-store")
	etx.Response().Header().Set("Pragma", "no-cache")

	resourceID, credentialID, err := parseChildIDs(etx, "credentialID")
	if err != nil {
		return etx.JSON(
			http.StatusNotFound,
			map[string]string{"error": "Resource credential not found"},
		)
	}
	var payload struct {
		Password string `json:"password"`
	}
	if err := etx.Bind(
		&payload,
	); err != nil || payload.Password == "" ||
		len(payload.Password) > 4096 {
		return etx.JSON(
			http.StatusUnprocessableEntity,
			map[string]string{"error": "Current password is required"},
		)
	}

	credential, err := controller.credentials.RevealManaged(
		etx.Request().Context(), resourceID, credentialID,
		cookies.ExtractFromCookieApp(etx).UserID, payload.Password,
	)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrInvalidCredentials):
			return etx.JSON(
				http.StatusUnprocessableEntity,
				map[string]string{"error": "Current password is incorrect"},
			)
		case errors.Is(err, services.ErrResourceCredentialUnavailable):
			return etx.JSON(
				http.StatusNotFound,
				map[string]string{"error": "Resource credential not found"},
			)
		default:
			slog.ErrorContext(
				etx.Request().Context(),
				"failed to reveal Resource credential",
				"resource_id",
				resourceID,
				"credential_id",
				credentialID,
				"error",
				err,
			)
			return etx.JSON(
				http.StatusInternalServerError,
				map[string]string{"error": "Resource credential could not be loaded"},
			)
		}
	}
	return etx.JSON(http.StatusOK, credential)
}
