package controllers

import (
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"deploycrate-ce/assets"
	"deploycrate-ce/config"
	"deploycrate-ce/database"
	"deploycrate-ce/database/seeds"
	"deploycrate-ce/internal/inertia"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models/factories"
	"deploycrate-ce/router/cookies"
	"deploycrate-ce/router/routes"

	"github.com/gorilla/sessions"
	"github.com/labstack/echo-contrib/v5/session"
	"github.com/labstack/echo/v5"
)

var controllerTestCluster *storage.TestCluster

func TestMain(m *testing.M) {
	gob.Register(cookies.FlashMessage{})

	rootHTML, err := assets.Files.ReadFile("inertia/root.go.html")
	if err != nil {
		fmt.Fprintf(os.Stderr, "read Inertia root template: %v\n", err)
		os.Exit(1)
	}
	viteManifest, err := assets.Files.ReadFile("dist/vite/manifest.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "read Vite manifest: %v\n", err)
		os.Exit(1)
	}
	if err := inertia.Init(
		config.ProjectName,
		config.Env,
		routes.ViteBuild.Path(),
		rootHTML,
		viteManifest,
	); err != nil {
		fmt.Fprintf(os.Stderr, "initialize Inertia: %v\n", err)
		os.Exit(1)
	}

	controllerTestCluster, err = storage.NewTestCluster(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "start Postgres test cluster: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	if err := controllerTestCluster.Close(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "close Postgres test cluster: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

func newControllerTestDB(t *testing.T, seed seeds.Runner) storage.Pool {
	t.Helper()

	db := controllerTestCluster.NewTestDB(t, database.Migrations, "migrations")
	if seed != nil {
		if err := seed(t.Context(), db.Executor()); err != nil {
			t.Fatalf("seed controller test database: %v", err)
		}
	}
	return db
}

func controllerTestConfig(t *testing.T) config.Config {
	t.Helper()
	return signInTestConfig(t, factories.TestPepper)
}

type controllerTestPage struct {
	Component string                     `json:"component"`
	Props     map[string]json.RawMessage `json:"props"`
}

func controllerRequest(
	t *testing.T,
	method string,
	target string,
	payload any,
	pathValues echo.PathValues,
	handler echo.HandlerFunc,
) *httptest.ResponseRecorder {
	t.Helper()

	var body bytes.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("encode controller request: %v", err)
		}
		body = *bytes.NewReader(encoded)
	}
	request := httptest.NewRequest(method, target, &body)
	request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	request.Header.Set("X-Inertia", "true")
	response := httptest.NewRecorder()
	etx := echo.New().NewContext(request, response)
	if pathValues != nil {
		etx.SetPathValues(pathValues)
	}

	store := sessions.NewCookieStore([]byte("01234567890123456789012345678901"))
	if err := session.Middleware(store)(handler)(etx); err != nil {
		t.Fatalf("run controller request: %v", err)
	}
	return response
}

func decodeControllerPage(t *testing.T, response *httptest.ResponseRecorder) controllerTestPage {
	t.Helper()

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusOK, response.Body)
	}
	var page controllerTestPage
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode Inertia page: %v; body: %s", err, response.Body)
	}
	return page
}

func requireControllerComponent(
	t *testing.T,
	response *httptest.ResponseRecorder,
	component string,
) controllerTestPage {
	t.Helper()

	page := decodeControllerPage(t, response)
	if page.Component != component {
		t.Fatalf("component = %q, want %q", page.Component, component)
	}
	return page
}
