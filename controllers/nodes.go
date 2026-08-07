package controllers

import (
	"errors"
	"net/http"
	"time"

	"deploycrate-ce/internal/inertia"
	"deploycrate-ce/internal/validation"
	"deploycrate-ce/models"
	"deploycrate-ce/router"
	"deploycrate-ce/router/cookies"
	"deploycrate-ce/router/middleware"
	"deploycrate-ce/router/routes"
	"deploycrate-ce/services"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

type Nodes struct {
	service *services.NodeEnrollment
}

func NewNodes(service *services.NodeEnrollment) Nodes {
	return Nodes{service: service}
}

func (controller Nodes) RegisterRoutes(r *router.Router) error {
	definitions := []struct {
		method string
		route  interface {
			Path() string
			Name() string
		}
		handler echo.HandlerFunc
	}{
		{http.MethodGet, routes.Nodes, controller.Index},
		{http.MethodGet, routes.NodeNew, controller.New},
		{http.MethodPost, routes.NodeCreate, controller.Create},
		{http.MethodGet, routes.NodeShow, controller.Show},
		{http.MethodPost, routes.NodeConfirm, controller.Confirm},
		{http.MethodPost, routes.NodeRetry, controller.Retry},
	}
	var errs []error
	for _, definition := range definitions {
		_, err := r.AddRoute(echo.Route{
			Method: definition.method, Path: definition.route.Path(), Name: definition.route.Name(),
			Handler: definition.handler, Middlewares: []echo.MiddlewareFunc{middleware.AdminOnly},
		})
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (controller Nodes) Index(etx *echo.Context) error {
	items, err := controller.service.List(etx.Request().Context())
	if err != nil {
		return inertia.Page(etx, "Errors/InternalError", inertia.Props{})
	}
	props := make([]inertia.Props, 0, len(items))
	for _, item := range items {
		props = append(props, nodeProps(item))
	}
	return inertia.Page(etx, "Nodes/Index", inertia.Props{"auth": authProps(etx), "nodes": props})
}

func (controller Nodes) New(etx *echo.Context) error {
	return controller.renderNew(etx)
}

func (controller Nodes) renderNew(etx *echo.Context, options ...inertia.PageOption) error {
	return inertia.Page(etx, "Nodes/New", inertia.Props{"auth": authProps(etx)}, options...)
}

type createNodePayload struct {
	Name         string                    `json:"name"`
	Address      string                    `json:"address"`
	Port         int                       `json:"port"`
	Username     string                    `json:"username"`
	PrivateKey   string                    `json:"privateKey"`
	Passphrase   string                    `json:"passphrase"`
	Capabilities models.ServerCapabilities `json:"capabilities"`
}

func (controller Nodes) Create(etx *echo.Context) error {
	var payload createNodePayload
	err := etx.Bind(&payload)
	var result services.NodeEnrollmentDetail
	if err == nil {
		result, err = controller.service.Create(etx.Request().Context(), services.CreateNodeInput{
			Name:         payload.Name,
			Address:      payload.Address,
			Port:         payload.Port,
			Username:     payload.Username,
			PrivateKey:   payload.PrivateKey,
			Passphrase:   payload.Passphrase,
			Capabilities: payload.Capabilities,
		})
	}
	if err != nil {
		if fieldErrors, ok := validation.As(err); ok {
			return controller.renderNew(etx, inertia.WithValidationErrors(fieldErrors.ToMap()))
		}
		_ = cookies.AddFlash(etx, cookies.FlashError, err.Error())
		return inertia.Redirect(etx, routes.NodeNew.URL(), http.StatusSeeOther)
	}
	return inertia.Redirect(etx, routes.NodeShow.URL(result.Enrollment.ID), http.StatusSeeOther)
}

func (controller Nodes) Show(etx *echo.Context) error {
	id, err := uuid.Parse(etx.Param("id"))
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	detail, err := controller.service.Get(etx.Request().Context(), id)
	if err != nil {
		return inertia.Page(etx, "Errors/NotFound", inertia.Props{})
	}
	return inertia.Page(
		etx,
		"Nodes/Show",
		inertia.Props{"auth": authProps(etx), "node": nodeProps(detail)},
	)
}

type confirmNodePayload struct {
	Fingerprint string `json:"fingerprint"`
}

func (controller Nodes) Confirm(etx *echo.Context) error {
	id, err := uuid.Parse(etx.Param("id"))
	var payload confirmNodePayload
	if err == nil {
		err = etx.Bind(&payload)
	}
	if err == nil {
		err = controller.service.Confirm(etx.Request().Context(), id, payload.Fingerprint)
	}
	if err != nil {
		_ = cookies.AddFlash(etx, cookies.FlashError, err.Error())
	} else {
		_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Node enrollment queued")
	}
	return inertia.Redirect(etx, routes.NodeShow.URL(id), http.StatusSeeOther)
}

func (controller Nodes) Retry(etx *echo.Context) error {
	id, err := uuid.Parse(etx.Param("id"))
	if err == nil {
		err = controller.service.Retry(etx.Request().Context(), id)
	}
	if err != nil {
		_ = cookies.AddFlash(etx, cookies.FlashError, err.Error())
	} else {
		_ = cookies.AddFlash(etx, cookies.FlashSuccess, "Node enrollment queued again")
	}
	return inertia.Redirect(etx, routes.NodeShow.URL(id), http.StatusSeeOther)
}

func nodeProps(detail services.NodeEnrollmentDetail) inertia.Props {
	var startedAt, completedAt, confirmedAt *time.Time
	var jobID *int64
	if detail.Enrollment.StartedAt.Valid {
		startedAt = &detail.Enrollment.StartedAt.Time
	}
	if detail.Enrollment.CompletedAt.Valid {
		completedAt = &detail.Enrollment.CompletedAt.Time
	}
	if detail.Credential.HostKeyConfirmedAt.Valid {
		confirmedAt = &detail.Credential.HostKeyConfirmedAt.Time
	}
	if detail.Enrollment.JobID.Valid {
		jobID = &detail.Enrollment.JobID.Int64
	}
	errorMessage := ""
	if detail.Enrollment.Error.Valid {
		errorMessage = detail.Enrollment.Error.String
	}
	capabilities, _ := models.ParseServerCapabilities(detail.Server.Capabilities)
	return inertia.Props{
		"id":                 detail.Enrollment.ID,
		"serverId":           detail.Server.ID,
		"name":               detail.Server.Name,
		"address":            detail.Server.Address,
		"sshPort":            detail.Credential.Port,
		"state":              detail.Enrollment.State,
		"currentStep":        detail.Enrollment.CurrentStep,
		"error":              errorMessage,
		"fingerprint":        detail.Enrollment.HostFingerprint,
		"hostKeyConfirmedAt": confirmedAt,
		"wireGuardAddress":   detail.Enrollment.AllocatedAddress,
		"installerVersion":   detail.Enrollment.InstallerVersion,
		"jobId":              jobID,
		"createdAt":          detail.Enrollment.CreatedAt,
		"startedAt":          startedAt,
		"completedAt":        completedAt,
		"configured":         detail.Server.IsConfigured,
		"capabilities":       capabilities,
	}
}
