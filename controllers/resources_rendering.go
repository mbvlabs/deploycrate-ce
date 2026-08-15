package controllers

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"deploycrate-ce/internal/inertia"
	"deploycrate-ce/internal/request"
	"deploycrate-ce/internal/validation"
	"deploycrate-ce/models"
	"deploycrate-ce/router/cookies"
	"deploycrate-ce/router/routes"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

func (controller Resources) renderShow(
	etx *echo.Context,
	resourceID uuid.UUID,
	option inertia.PageOption,
) error {
	return controller.renderShowSection(etx, resourceID, resourceReturnSection(etx), nil, option)
}

func (controller Resources) renderShowSection(
	etx *echo.Context,
	resourceID uuid.UUID,
	section string,
	enrollment inertia.Props,
	option inertia.PageOption,
) error {
	if err := controller.access.ObserveResource(etx.Request().Context(), resourceID); err != nil {
		slog.WarnContext(
			etx.Request().Context(),
			"failed to observe Resource WireGuard device handshakes",
			"resource_id",
			resourceID,
			"error",
			err,
		)
	}
	detail, err := controller.service.Details(etx.Request().Context(), resourceID)
	if errors.Is(err, models.ErrNotFound) {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	if err != nil {
		return controller.renderLoadError(etx, err)
	}
	publications, err := controller.caddy.ResourcePublications(etx.Request().Context(), resourceID)
	if err != nil {
		return controller.renderLoadError(etx, err)
	}
	dnsZones, err := controller.caddy.ResourceDNSOptions(etx.Request().Context())
	if err != nil {
		return controller.renderLoadError(etx, err)
	}
	options, err := controller.service.OptionsForEngine(
		etx.Request().Context(),
		detail.Resource.Engine(),
	)
	if err != nil {
		return controller.renderLoadError(etx, err)
	}
	privateAccess, err := controller.access.Details(
		etx.Request().Context(),
		resourceID,
		cookies.ExtractFromCookieApp(etx).UserID,
	)
	if err != nil {
		return controller.renderLoadError(etx, err)
	}
	backups := models.ResourceBackupCatalog{}
	if section == "backups" || section == "databases" {
		backups, err = controller.backups.DetailsForResource(etx.Request().Context(), resourceID)
		if err != nil {
			return controller.renderLoadError(etx, err)
		}
	}
	props := inertia.Props{
		"auth":             authProps(etx),
		"resource":         resourceDetailProps(detail, privateAccess),
		"backups":          resourceBackupProps(backups),
		"options":          resourceOptionsProps(options),
		"publications":     publications,
		"dnsZones":         dnsZones,
		"section":          section,
		"selectedDatabase": strings.TrimSpace(etx.QueryParam("database")),
		"flash":            resourceFlashProps(etx),
	}
	if enrollment != nil {
		props["enrollment"] = enrollment
	}
	if option != nil {
		return inertia.Page(etx, "Resources/Show", props, option)
	}
	return inertia.Page(etx, "Resources/Show", props)
}

func (controller Resources) renderEdit(
	etx *echo.Context,
	resourceID uuid.UUID,
	option inertia.PageOption,
) error {
	detail, err := controller.service.Details(etx.Request().Context(), resourceID)
	if errors.Is(err, models.ErrNotFound) {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	if err != nil {
		return controller.renderLoadError(etx, err)
	}
	options, err := controller.service.Options(etx.Request().Context())
	if err != nil {
		return controller.renderLoadError(etx, err)
	}
	props := inertia.Props{
		"auth":     authProps(etx),
		"resource": resourceDetailProps(detail, models.ResourcePrivateAccessDetails{}),
		"options":  resourceOptionsProps(options),
		"flash":    resourceFlashProps(etx),
	}
	if option != nil {
		return inertia.Page(etx, "Resources/Edit", props, option)
	}
	return inertia.Page(etx, "Resources/Edit", props)
}

func (controller Resources) renderCreateError(etx *echo.Context, err error) error {
	options, optionsErr := controller.service.Options(etx.Request().Context())
	if optionsErr != nil {
		return controller.renderLoadError(etx, errors.Join(err, optionsErr))
	}
	if validationErrors, ok := validation.As(err); ok {
		return inertia.Page(
			etx,
			"Resources/New",
			inertia.Props{"auth": authProps(etx), "options": resourceOptionsProps(options)},
			inertia.WithValidationErrors(validationErrors.ToMap()),
		)
	}
	return controller.redirectError(etx, routes.ResourceNew.URL(), err)
}

func (controller Resources) finishChildMutation(
	etx *echo.Context,
	resourceID uuid.UUID,
	err error,
	success string,
) error {
	section := resourceReturnSection(etx)
	if err != nil {
		if validationErrors, ok := validation.As(err); ok && resourceID != uuid.Nil {
			if section == "settings" {
				return controller.renderEdit(
					etx,
					resourceID,
					inertia.WithValidationErrors(validationErrors.ToMap()),
				)
			}
			return controller.renderShowSection(
				etx,
				resourceID,
				section,
				nil,
				inertia.WithValidationErrors(validationErrors.ToMap()),
			)
		}
		return controller.redirectError(etx, resourceSectionURL(resourceID, section), err)
	}
	if flashErr := cookies.AddFlash(etx, cookies.FlashSuccess, success); flashErr != nil {
		return controller.renderLoadError(etx, flashErr)
	}
	return inertia.Redirect(etx, resourceSectionURL(resourceID, section), http.StatusSeeOther)
}

func resourceReturnSection(etx *echo.Context) string {
	section := etx.QueryParam("returnTo")
	if section == "edit" {
		return "settings"
	}
	switch section {
	case "databases", "backups", "endpoints", "credentials", "health", "settings":
		return section
	default:
		return "overview"
	}
}

func resourceSectionURL(resourceID uuid.UUID, section string) string {
	switch section {
	case "databases":
		return routes.ResourceDatabases.URL(resourceID)
	case "backups":
		return routes.ResourceBackups.URL(resourceID)
	case "endpoints":
		return routes.ResourceEndpoints.URL(resourceID)
	case "credentials":
		return routes.ResourceCredentials.URL(resourceID)
	case "health":
		return routes.ResourceHealth.URL(resourceID)
	case "settings":
		return routes.ResourceSettings.URL(resourceID)
	default:
		return routes.ResourceShow.URL(resourceID)
	}
}

func resourceFlashProps(etx *echo.Context) []inertia.Props {
	flashes := request.ExtractContext[[]cookies.FlashMessage](
		etx.Request().Context(),
		request.SessionFlashesKey,
	)
	props := make([]inertia.Props, 0, len(flashes))
	for _, flash := range flashes {
		props = append(props, inertia.Props{"type": flash.Type, "message": flash.Message})
	}
	return props
}

func (controller Resources) redirectError(etx *echo.Context, location string, err error) error {
	message := "Resource operation failed"
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		message = err.Error()
	}
	if flashErr := cookies.AddFlash(etx, cookies.FlashError, message); flashErr != nil {
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	return inertia.Redirect(etx, location, http.StatusSeeOther)
}

func (controller Resources) renderLoadError(etx *echo.Context, err error) error {
	slog.ErrorContext(etx.Request().Context(), "failed to load Resource page", "error", err)
	return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
}

func parseChildIDs(etx *echo.Context, childParam string) (uuid.UUID, uuid.UUID, error) {
	resourceID, resourceErr := uuidPathParam(etx, "id")
	childID, childErr := uuidPathParam(etx, childParam)
	return resourceID, childID, errors.Join(resourceErr, childErr)
}

func parseResourceDatabasePolicyIDs(etx *echo.Context) (uuid.UUID, string, uuid.UUID, error) {
	resourceID, resourceErr := uuidPathParam(etx, "id")
	databaseName := strings.TrimSpace(etx.Param("databaseName"))
	var databaseErr error
	if databaseName == "" {
		databaseErr = errors.New("database is required")
	}
	policyID, policyErr := uuidPathParam(etx, "backupPolicyID")
	return resourceID, databaseName, policyID, errors.Join(resourceErr, databaseErr, policyErr)
}

func bindResourceChild[T any](etx *echo.Context, bind func() (T, error)) (uuid.UUID, T, error) {
	resourceID, err := uuidPathParam(etx, "id")
	var input T
	if err == nil {
		input, err = bind()
	}
	return resourceID, input, err
}

func domainPayloadError(field, message string) error {
	return errors.Join(
		models.ErrDomainValidation,
		validation.ValidationErrors{{Field: field, Code: "invalid", Message: message}},
	)
}

func prefixResourceValidation(err error, prefix string) error {
	validationErrors, ok := validation.As(err)
	if !ok {
		return err
	}
	return errors.Join(
		models.ErrDomainValidation,
		validation.WithFieldPrefix(validationErrors, prefix),
	)
}
