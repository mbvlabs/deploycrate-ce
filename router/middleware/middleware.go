// Package middleware provides HTTP middleware for the Echo web framework,
package middleware

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"deploycrate-ce/config"
	"deploycrate-ce/internal/request"
	"deploycrate-ce/internal/server"
	"deploycrate-ce/router/cookies"
	"deploycrate-ce/router/routes"
	"deploycrate-ce/telemetry"

	"github.com/labstack/echo/v5"
	echomw "github.com/labstack/echo/v5/middleware"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

func RegisterRequestMeta(
	next echo.HandlerFunc,
) echo.HandlerFunc {
	return func(c *echo.Context) error {
		if isAssetsPath(c.Request().URL.Path) || isAPIPath(c.Request().URL.Path) ||
			isGitHubWebhookPath(c.Request().URL.Path) {
			return next(c)
		}

		appCookie := cookies.ExtractFromCookieApp(c)
		appCookie.CurrentPath = c.Request().URL.Path

		flashes, err := cookies.ExtractFlashes(c)
		if err != nil {
			slog.Error("Error getting flash messages from session", "error", err)
		}

		returnTo := cookies.GetReturnTo(c)

		method := c.Request().Method
		if method == http.MethodGet || method == http.MethodHead {
			referer := strings.TrimSpace(c.Request().Referer())
			if referer == "" {
				if sessErr := cookies.SetReturnTo(c, ""); sessErr != nil {
					slog.Warn("Error clearing return_to", "error", sessErr)
				}
				returnTo = ""
			} else {
				refererURL, parseErr := url.Parse(referer)
				if parseErr != nil {
					if sessErr := cookies.SetReturnTo(c, ""); sessErr != nil {
						slog.Warn("Error clearing return_to", "error", sessErr)
					}
					returnTo = ""
				} else if refererURL.Host != "" && !strings.EqualFold(refererURL.Host, c.Request().Host) {
					if sessErr := cookies.SetReturnTo(c, ""); sessErr != nil {
						slog.Warn("Error clearing return_to", "error", sessErr)
					}
					returnTo = ""
				} else {
					newReturnTo := refererURL.EscapedPath()
					if refererURL.RawQuery != "" {
						newReturnTo += "?" + refererURL.RawQuery
					}
					if newReturnTo == "" {
						newReturnTo = "/"
					}

					current := c.Request().URL.Path
					if c.Request().URL.RawQuery != "" {
						current += "?" + c.Request().URL.RawQuery
					}

					if newReturnTo == current ||
						!strings.HasPrefix(newReturnTo, "/") ||
						strings.HasPrefix(newReturnTo, "//") {
						if sessErr := cookies.SetReturnTo(c, ""); sessErr != nil {
							slog.Warn("Error clearing return_to", "error", sessErr)
						}
						returnTo = ""
					} else {
						if sessErr := cookies.SetReturnTo(c, newReturnTo); sessErr != nil {
							slog.Warn("Error setting return_to", "error", sessErr)
						}
						returnTo = newReturnTo
					}
				}
			}
		}

		ctx := request.BuildRequestMeta(c.Request().Context(), map[request.AppContextKey]any{
			request.SessionCookieKey:  appCookie,
			request.SessionFlashesKey: flashes,
			request.BackURLKey:        returnTo,
		})

		c.SetRequest(c.Request().WithContext(ctx))

		return next(c)
	}
}

func ValidateSession(
	next echo.HandlerFunc,
) echo.HandlerFunc {
	return func(c *echo.Context) error {
		// Skip session validation for static assets and API routes
		if isAssetsPath(c.Request().URL.Path) || isAPIPath(c.Request().URL.Path) ||
			isGitHubWebhookPath(c.Request().URL.Path) {
			return next(c)
		}
		if err := cookies.RecoverInvalidSessions(c); err != nil {
			return err
		}

		return next(c)
	}
}

func isAPIPath(path string) bool {
	return matchesPathPrefix(path, routes.APIPrefix)
}

func isAssetsPath(path string) bool {
	return matchesPathPrefix(path, routes.AssetsPrefix)
}

func isGitHubWebhookPath(path string) bool {
	return path == routes.GitHubWebhook.Path()
}

func matchesPathPrefix(path, prefix string) bool {
	prefix = strings.TrimSuffix(prefix, "/")
	if prefix == "" {
		return false
	}

	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

func hasNonEmptyBearerToken(authorization string) bool {
	parts := strings.Fields(authorization)
	return len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") && parts[1] != ""
}

func hasApplicationSessionCookie(request *http.Request) bool {
	_, err := request.Cookie(config.AppCookieSessionName)
	return err == nil
}

func mayBypassCSRF(request *http.Request) bool {
	if isGitHubWebhookPath(request.URL.Path) {
		return true
	}
	return isAPIPath(request.URL.Path) &&
		hasNonEmptyBearerToken(request.Header.Get("Authorization")) &&
		!hasApplicationSessionCookie(request)
}

func Logger(tel *telemetry.Telemetry) echo.MiddlewareFunc {
	var httpInFlight metric.Int64UpDownCounter

	if tel.HasMetrics() {
		var err error
		httpInFlight, err = telemetry.HTTPRequestsInFlight()
		if err != nil {
			slog.Warn("failed to create http.server.active_requests metric", "error", err)
		}
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if isAssetsPath(c.Request().URL.Path) ||
				c.Request().URL.Path == routes.SystemTelemetryLogs.Path() ||
				c.Request().URL.Path == routes.Health.Path() {
				return next(c)
			}

			ctx := c.Request().Context()
			start := time.Now()

			if tel.HasMetrics() && httpInFlight != nil {
				attributes := metric.WithAttributes(
					attribute.String("http.request.method", c.Request().Method),
				)
				httpInFlight.Add(ctx, 1, attributes)
				defer httpInFlight.Add(ctx, -1, attributes)
			}

			err := next(c)
			duration := time.Since(start)

			statusCode := 0
			if resp, unwrapErr := echo.UnwrapResponse(c.Response()); unwrapErr == nil {
				statusCode = resp.Status
			}
			if statusCode == 0 {
				statusCode = http.StatusOK
				var httpError *echo.HTTPError
				if errors.As(err, &httpError) {
					statusCode = httpError.Code
				} else if err != nil {
					statusCode = http.StatusInternalServerError
				}
			}

			slog.Log(ctx, requestLogLevel(statusCode), "HTTP request completed",
				"method", c.Request().Method,
				"path", c.Request().URL.Path,
				"status", statusCode,
				"remote_addr", c.RealIP(),
				"user_agent", c.Request().UserAgent(),
				"duration", duration.String(),
			)

			return err
		}
	}
}

func requestLogLevel(statusCode int) slog.Level {
	switch {
	case statusCode >= http.StatusInternalServerError:
		return slog.LevelError
	case statusCode >= http.StatusBadRequest:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}

func TraceRouteAttributes(tel *telemetry.Telemetry) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if isAssetsPath(c.Request().URL.Path) {
				return next(c)
			}

			err := next(c)
			routeInfo := c.RouteInfo()
			if routeInfo.Path == "" {
				return err
			}
			ctx := c.Request().Context()
			if labeler, ok := otelhttp.LabelerFromContext(ctx); ok {
				labeler.Add(semconv.HTTPRoute(routeInfo.Path))
			}
			if !tel.HasTracing() {
				return err
			}

			span := trace.SpanFromContext(ctx)
			if !span.SpanContext().IsValid() {
				return err
			}

			span.SetName(c.Request().Method + " " + routeInfo.Path)
			span.SetAttributes(semconv.HTTPRoute(routeInfo.Path))

			return err
		}
	}
}

func CSRFMiddleware(cfg config.Config, csrfName string) (echo.MiddlewareFunc, error) {
	strategy := strings.TrimSpace(cfg.App.CSRFStrategy)

	var headerOnly bool
	var tokenLookup string
	switch strategy {
	case "header_only":
		headerOnly = true
		tokenLookup = "cookie:" + csrfName
	case "header_or_legacy_token":
		headerOnly = false
		tokenLookup = "header:X-CSRF-Token,form:_csrf"
	default:
		return nil, errors.New("invalid CSRF strategy")
	}

	trustedOrigins := []string{config.BaseURL}
	if len(cfg.App.CSRFTrustedOrigins) > 0 {
		trustedOrigins = append(trustedOrigins, cfg.App.CSRFTrustedOrigins...)
	}

	csrfConfig := echomw.CSRFConfig{
		Skipper: func(c *echo.Context) bool {
			return mayBypassCSRF(c.Request())
		},
		TokenLookup: tokenLookup,
		CookiePath:  "/",
		CookieDomain: func() string {
			if config.Env == server.ProdEnvironment {
				return config.Domain
			}

			return ""
		}(),
		CookieSecure:   config.Env == server.ProdEnvironment,
		CookieHTTPOnly: true,
		CookieSameSite: http.SameSiteStrictMode,
		TrustedOrigins: trustedOrigins,
	}

	echoCSRF := echomw.CSRFWithConfig(csrfConfig)

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if mayBypassCSRF(c.Request()) {
				return next(c)
			}

			// Add Vary header for proper caching behavior
			c.Response().Header().Add("Vary", "Sec-Fetch-Site")

			method := c.Request().Method
			isUnsafe := method != http.MethodGet && method != http.MethodHead &&
				method != http.MethodOptions && method != http.MethodTrace

			if isUnsafe {
				secFetchSite := strings.ToLower(
					strings.TrimSpace(c.Request().Header.Get("Sec-Fetch-Site")),
				)

				// In header_only mode, reject requests missing Sec-Fetch-Site
				if headerOnly && (secFetchSite == "" || secFetchSite == "none") {
					return echo.NewHTTPError(
						http.StatusForbidden,
						"CSRF verification failed: missing Sec-Fetch-Site header",
					)
				}

				// In legacy mode, log when falling back to form token
				if !headerOnly && secFetchSite != "same-origin" && secFetchSite != "same-site" &&
					secFetchSite != "cross-site" {
					if c.Request().Header.Get("X-CSRF-Token") == "" && c.FormValue("_csrf") != "" {
						slog.Warn("CSRF check fell back to legacy token")
					}
				}
			}

			// Delegate to Echo's CSRF middleware
			return echoCSRF(next)(c)
		}
	}, nil
}
