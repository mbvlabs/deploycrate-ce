# deploycrate-ce

A full-stack web application built with [Andurel](https://github.com/mbvlabs/andurel), a Rails-like web framework for Go that prioritizes development speed.

## VPS Bootstrap CLI

This repository contains the interactive `deploycrate` CLI for configuring a fresh Debian 13 VPS as a single-server DeployCrate CE host. Bootstrap is supported for fresh installations and interrupted-installation recovery only. It does not upgrade an existing DeployCrate CE installation.

### Host Requirements

A non-dry-run installation must be started as root from an interactive terminal. Preflight accepts Debian 13 on AMD64 or ARM64 and requires:

- `apt-get`, Bash, systemd, and OpenSSH server.
- At least 512 MB of available memory and 5,120 MB free on the root filesystem.
- Outbound HTTPS access for Debian packages, Docker, Caddy, Buildpacks, and release assets.
- A public domain pointed at the server, plus any provider-level firewall rules needed for the selected SSH port, TCP ports 80 and 443, and UDP port 51820.

The installer changes SSH access near the end of bootstrap. Root login and password authentication are disabled, and only the `deploycrate` user is allowed. Keep the original SSH session open until the generated handoff command has been verified from a second terminal.

### Install a Release

The released installer is intended to be run as root from an interactive SSH session:

```bash
curl -fsSL https://get.deploycrate.com/ce | sudo bash
```

The shell installer downloads the release archive, verifies its SHA-256 checksum, verifies its Sigstore bundle when `cosign` is available, installs `deploycrate` and `deploycrate-ce` under `/usr/local/bin`, and opens the wizard through `/dev/tty`. Without a usable TTY it installs the binaries and prints the command needed to continue. GitHub Releases is the default asset source. `DEPLOYCRATE_VERSION`, `DEPLOYCRATE_RELEASE_REPOSITORY`, and `DEPLOYCRATE_RELEASE_BASE_URL` can select a version or compatible release mirror.

For the current AMD64 development build:

```bash
curl -fsSL https://get-dev.deploycrate.com/install.sh | sudo bash
```

The development installer verifies the CLI checksum and installs the CLI first. During bootstrap, the CLI downloads and verifies the matching application binary from the same development endpoint. `DEPLOYCRATE_DEVELOPMENT_BASE_URL` can select a compatible mirror. Maintainers publish both development binaries, checksums, and the installer with `just development-assets`; the recipe synchronizes the resulting directory to `dc-ce-dev:deploycrate-development`.

### Wizard Inputs

The wizard collects and reviews:

| Setting | Behavior |
| --- | --- |
| Domain and SSH port | The domain is entered without a protocol. The SSH port defaults to `22`. |
| Operating-system access | The `deploycrate` Linux password must contain at least 12 characters. Supply one SSH public key or leave it blank to generate an Ed25519 key pair for one-time handoff. |
| Application administrator | The wizard requires a valid email address and a password of at least 8 characters. The administrator is created or updated and marked verified. |
| PostgreSQL | Choose a local PostgreSQL 17 Docker container or an external server. External connections support `disable`, `require`, `verify-ca`, and `verify-full`; an optional CA file is copied to a managed path before installation starts. |
| Backup destination | Optional S3-compatible endpoint, region, bucket, and credentials. The values are stored, but backup execution and S3 connectivity validation are not implemented yet. |

Generated session, encryption, signing, pepper, and local database secrets are not prompted for or printed. Database credentials remain in the protected application environment file until the resource credential encryption contract is implemented.

### Bootstrap Behavior

After the operator approves the review screen, the CLI saves resumable configuration and runs these phases in order:

1. Installs baseline packages and creates the `deploycrate` user with Docker access and unrestricted passwordless sudo.
2. Configures persistent journald storage, fail2ban, and a 1 GB `/swapfile` only when the host has no active swap.
3. Installs WireGuard tools; creates a root-only keypair and `wg0` configuration; assigns `10.99.0.1/16`; listens on UDP `51820`; opens UFW; and enables and verifies `wg-quick@wg0`.
4. Installs and configures Docker Engine, then installs checksum-verified Buildpacks `pack` 0.40.6 and creates the deploycrate-owned build workspace. It does not pre-pull a builder image.
5. Starts local PostgreSQL or verifies the external connection, installs the application release, writes protected runtime configuration, applies embedded migrations, creates or updates the administrator, and stores optional backup settings.
6. Creates blue and green systemd slots on `127.0.0.1:8080` and `127.0.0.1:8081`, but links and starts only the initial blue slot.
7. Installs checksum-verified Caddy 2.11.4, verifies weighted round-robin support, holds that package version, and verifies the application at `http://127.0.0.1:8080/api/health`.
8. Records the application, production environment, server, WireGuard network and peer, database resource, bootstrap change, release, deployment, blue instance, domain, Caddy route, and 100-weight blue route backend in PostgreSQL. The WireGuard private key is stored there only after AES-256-GCM encryption.
9. Initializes Caddy's `srv0` HTTP server when its configuration is empty, then applies the route through the local API. Other Caddy `400` responses remain installation errors. It configures Caddy to resume its autosaved API configuration after reboot, hardens SSH, and enables UFW for the selected SSH port, TCP 80 and 443, and UDP 51820.
10. Displays the application, SSH, and administrator credentials. Typing `CONFIRM` records the handoff, permanently removes the transient installer secrets file, and reboots the server. The WireGuard private key is not part of this handoff.

The health check retries for about one minute. A single-server WireGuard mesh has no handshake until another peer joins.

### CLI Commands and Recovery

| Command | Behavior |
| --- | --- |
| `sudo deploycrate install` | Opens the wizard for a fresh host. It rejects resumable, completed, or inconsistent installer state. |
| `sudo deploycrate install --dry-run` | Walks through the complete wizard and setup phases without preflight enforcement, persistent state, host mutation, secret cleanup, or reboot. A TTY is still required. |
| `sudo deploycrate resume` | Loads the saved configuration, skips steps already marked complete, reruns failed or incomplete steps, and returns to credential handoff. Use this after fixing the reported failure. |
| `sudo deploycrate resume --dry-run` | Reads an existing resumable configuration and previews every setup phase without mutation. A TTY is still required. |
| `sudo deploycrate logs` | Prints `/var/lib/deploycrate-ce/install.log`. Script output is redacted using the collected secret values. |
| `deploycrate version` | Prints the CLI version. `deploycrate --version` and `deploycrate -v` are aliases. |
| `deploycrate help` | Prints command usage. Running without arguments, `deploycrate --help`, and `deploycrate -h` do the same. |
| `sudo deploycrate doctor [--json]` | Accepted command, but currently exits with a not-implemented error. JSON output is not implemented. |
| `sudo deploycrate backup` | Accepted command, but currently exits with a not-implemented error. |

Configuration is saved before the first setup phase and each completed step is recorded in `/var/lib/deploycrate-ce/install-state.json`. A non-blocking process lock prevents concurrent installers. If setup fails after configuration is saved, fix the reported problem and run `sudo deploycrate resume`; do not start a second installation. The topology transaction is reused by domain if Caddy reconciliation needs to be retried.

If credential verification was recorded but secret cleanup failed, `resume` returns directly to the final handoff. If cleanup succeeded but the reboot command failed, the installation is complete and `resume` is rejected; reboot the host manually.

The installer does not create an emergency user. Node-exporter is also not installed: there is no node-exporter binary, systemd unit, port `9100` listener, or bootstrap health check.

### Installed Host Layout

DeployCrate separates bootstrap commands, immutable application releases, slot pointers, protected configuration, and mutable runtime state. The application service never executes the bootstrap copy in `/usr/local/bin` directly.

```text
/usr/local/bin/
|-- deploycrate                         Bootstrap and recovery CLI
|-- deploycrate-ce                      Production installer payload
`-- pack                                Cloud Native Buildpacks CLI

/opt/deploycrate-ce/
|-- releases/
|   `-- <version>/deploycrate-ce        Immutable installed application release
`-- slots/
    |-- blue/deploycrate-ce  -> /opt/deploycrate-ce/releases/<blue-version>/deploycrate-ce
    `-- green/deploycrate-ce -> /opt/deploycrate-ce/releases/<green-version>/deploycrate-ce
                               Created when the first update is staged
```

The initial installation creates the blue and green slot directories, installs the selected application release under `/opt/deploycrate-ce/releases/<version>/`, points the blue slot at that release, and starts only `deploycrate-ce@blue.service`. A slot path is an atomic symlink to a release binary. It identifies what that slot will execute, but it does not by itself mean the slot is active.

The remaining DeployCrate-managed locations are:

| Location | Contents and ownership |
| --- | --- |
| `/etc/deploycrate-ce/` | Root-owned application configuration. `app.env` contains runtime configuration and secrets, `backup.env` contains optional backup destination settings, and `slots/blue.env` plus `slots/green.env` assign ports 8080 and 8081. |
| `/etc/deploycrate-ce/installer.json` | Durable non-secret installer configuration used by `resume`. |
| `/etc/deploycrate-ce/installer-secrets.json` | Transient installer credentials. This file is removed only after the operator types `CONFIRM` at the final handoff. |
| `/etc/ssl/certs/deploycrate-ce-postgresql-ca.crt` | Managed copy of an external PostgreSQL CA certificate, when one was supplied. It remains readable by the application service. |
| `/etc/wireguard/` | Root-only `deploycrate-ce.key`, `deploycrate-ce.pub`, and `wg0.conf` files for the initial mesh peer. |
| `/var/lib/deploycrate-ce/` | Root-owned installer state, including `install-state.json`, `install.lock`, and the redacted `install.log`. |
| `/var/lib/deploycrate-ce/runtime/` | Mutable application runtime state owned by the `deploycrate` user, including `self-update.json`. |
| `/var/lib/deploycrate-builds/` | Build workspace owned by the `deploycrate` user. Builder images and build containers remain Docker-managed. |
| `/home/deploycrate/.ssh/authorized_keys` | SSH access for the `deploycrate` operating-system user. |

The installer also places the checksum-verified Buildpacks CLI at `/usr/local/bin/pack`. Caddy is installed from the pinned official Debian package at `/usr/bin/caddy` and held at the installer-supported version. Docker Engine and the remaining host packages use their standard Debian package locations.

When local PostgreSQL is selected, its data is stored in the Docker named volume `deploycrate-ce-postgres`, mounted at `/var/lib/postgresql/data` inside the `deploycrate-ce-postgres` container. The physical host path belongs to Docker and can be found with `docker volume inspect deploycrate-ce-postgres`.

The installer also writes host integration files outside the DeployCrate directories:

- `/etc/systemd/system/deploycrate-ce@.service` defines both application slots.
- `/etc/wireguard/wg0.conf` is managed by `wg-quick@wg0.service` and contains the live WireGuard interface configuration.
- `/etc/systemd/system/caddy.service.d/deploycrate-ce.conf` makes Caddy resume its autosaved API configuration after reboot.
- `/etc/caddy/Caddyfile` enables Caddy's local administration endpoint.
- `/etc/systemd/journald.conf.d/deploycrate-ce.conf`, `/etc/fail2ban/jail.d/deploycrate-ce.conf`, and `/etc/ssh/sshd_config.d/99-deploycrate-ce.conf` configure host logging and SSH protection.
- `/etc/sudoers.d/deploycrate` grants the documented passwordless sudo access.
- `/etc/docker/daemon.json` configures Docker log rotation.
- `/swapfile` is created only when the host has no active swap.

### Post-install Inspection

Application and Caddy:

```bash
sudo systemctl status deploycrate-ce@blue.service
curl -fsS http://127.0.0.1:8080/api/health
curl -fsS http://127.0.0.1:2019/config/
curl -fsS http://127.0.0.1:2019/config/apps/http/servers/srv0/routes
```

WireGuard:

```bash
sudo systemctl status wg-quick@wg0
sudo wg show wg0
ip address show wg0
sudo cat /etc/wireguard/deploycrate-ce.pub
```

Local PostgreSQL:

```bash
sudo docker exec -it deploycrate-ce-postgres \
  psql --username deploycrate --dbname deploycrate_ce
```

### How the Active Slot Is Determined

Blue listens on `127.0.0.1:8080` and green listens on `127.0.0.1:8081`. Before a self-update starts, DeployCrate requires two independent views of the active slot to agree:

```text
Persisted topology
backend with weight 100 -> instance.slot: blue or green
                         |
                         | must match
                         v
Observed host state
exactly one active systemd unit: deploycrate-ce@blue or deploycrate-ce@green
                         |
                         v
Caddy sends public traffic to the 100-weight slot
```

DeployCrate queries `systemctl is-active` for both slot services. It refuses to update if both are running, neither is running, or the running service disagrees with the active slot recorded by the 100-weight database backend.

During an update, DeployCrate installs the verified binary in a new immutable release directory, repoints only the inactive slot symlink, starts that slot, and checks its internal health endpoint. It then switches Caddy weights from `100/0` to `0/100`, verifies the public health endpoint, enables the new systemd unit for the next boot, disables the previous unit, and finally stops the previous slot. Any failure before completion restores traffic, service enablement, and the previous inactive-slot symlink.

An operator can inspect the host-side state with:

```bash
sudo systemctl is-active deploycrate-ce@blue.service
sudo systemctl is-active deploycrate-ce@green.service
sudo systemctl is-enabled deploycrate-ce@blue.service
sudo systemctl is-enabled deploycrate-ce@green.service
readlink -f /opt/deploycrate-ce/slots/blue/deploycrate-ce
readlink -f /opt/deploycrate-ce/slots/green/deploycrate-ce
```

The green `readlink` command has no target until the first update has staged a release into that slot.

## Project Structure

```
deploycrate-ce/
├── assets/              # Static assets (compiled CSS, images)
├── bin/                 # Command-line tools
│   ├── app              # Main application binary
│   ├── console          # Database console
│   ├── migration        # Migration runner
│   └── shadowfax        # Development server
├── cmd/                 # Command entry points
│   └── app/             # Main web application
├── clients/             # External service clients
├── config/              # Application configuration
├── controllers/         # HTTP request handlers
├── css/                 # Source CSS files (Tailwind wrappers + theme)
├── examples/
│   └── html/            # Copy/paste HTML snippets with Datastar attributes
├── database/
│   └── migrations/      # SQL migration files
├── email/               # Email templates and sending
├── models/              # Data models and business logic
├── queue/               # Background job processing
│   ├── jobs/            # Job definitions
│   └── workers/         # Worker implementations
├── router/              # Routes and middleware
│   ├── routes/          # Route definitions
│   ├── cookies/         # Session helpers
│   └── middleware/      # Custom middleware
├── pkg/
│ 	└──telemetry/        # Observability (logs, traces, metrics)
├── views/               # Templ templates
├── .env.example         # Example environment configuration
└── go.mod               # Go dependencies
```

## Quick Start

### Prerequisites

- Go 1.24.4 or higher
- PostgreSQL database
- Andurel CLI: `go install github.com/mbvlabs/andurel@latest`

### Setup

1. **Configure environment**
   ```bash
   cp .env.example .env
   # Edit .env with your configuration
   ```

2. **Create database**
   ```bash
   createdb deploycrate-ce_development
   ```

3. **Run migrations**
   ```bash
   andurel migration up
   ```

4. **Start the development server**
   ```bash
   andurel run
   ```

Your application is now running at `http://localhost:8080` with live reload for Go, Templ, and CSS changes!

## Available Commands

### Development Server

```bash
# Run development server with hot reload for Go, Templ, and CSS
andurel run
```

This orchestrates Air (Go), Templ watch, and Tailwind CSS compilation.

### Database Console

```bash
# Open interactive database console
andurel app console
```

Provides a SQL console connected to your database for ad-hoc queries and exploration.

### Migration Management

```bash
# Create a new migration
andurel migration new create_users_table

# Run all pending migrations
andurel migration up

# Rollback last migration
andurel migration down

# Rollback to specific version
andurel migration down-to [version]

# Apply up to specific version
andurel migration up-to [version]

# Reset database (rollback all, then reapply)
andurel migration reset

# Fix migration version gaps
andurel migration fix
```

## How-To Guides

### Generate a New Resource

The Andurel generator creates complete CRUD resources with models, controllers, views, and routes.

**Prerequisites**: You need a database table first. Create a migration:

```bash
# 1. Create a migration for your table
andurel migration new create_products_table
```

Edit the generated migration file in `database/migrations/` to define your table schema:

```sql
-- +goose Up
CREATE TABLE products (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    price DECIMAL(10, 2) NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

-- +goose Down
DROP TABLE products;
```

Apply the migration:

```bash
andurel migration up
```

**Generate the resource**:

```bash
# Generate model + controller + views + routes
andurel generate resource Product

# Or use shorthand
andurel g resource Product
```

This creates:
- `models/product.go` - Data model with CRUD methods
- `controllers/products.go` - HTTP handlers for CRUD operations
- `views/products_*.templ` - Template files for all CRUD views
- Routes automatically registered in `router/routes/products.go`

The generator also:
- Creates Bun-backed model methods for CRUD operations
- Creates a complete CRUD interface at `/products`

**Custom table names**: If your table doesn't follow Rails naming conventions (model `Product` → table `products`):

```bash
# Map Product model to a custom table name
andurel g resource Product --table products_catalog
```

**Individual components**:

```bash
# Generate only the model
andurel g model Product

# Generate controller with views
andurel g controller Product --with-views

# Generate views with controller
andurel g view Product --with-controller

# Refresh model after schema changes
andurel g model Product --refresh
```

### Setup Background Jobs

This project uses [River](https://riverqueue.com/) for background job processing with PostgreSQL.

**1. Define a job**

Create a new job type in `queue/jobs/`:

```go
// queue/jobs/my_job.go
package jobs

type MyJobArgs struct {
    UserID   string
    Action   string
}

func (MyJobArgs) Kind() string { return "my_job" }
```

**2. Implement a worker**

Create the worker in `queue/workers/`:

```go
// queue/workers/my_job.go
package workers

import (
    "context"
    "deploycrate-ce/queue/jobs"
)

func ProcessMyJob(ctx context.Context, msg []byte) error {
    // Your job logic here
    // Unmarshal msg to jobs.MyJobArgs and process
    return nil
}
```

**3. Register the worker**

Add your worker to `queue/workers/workers.go`:

```go
// Register in your queue setup
```

**4. Enqueue jobs**

From anywhere in your application:

```go
import "deploycrate-ce/queue/jobs"

// Enqueue a job through your queue client
err := queue.Enqueue(ctx, jobs.MyJobArgs{
    UserID: "123",
    Action: "send_welcome_email",
})
```


**Job Options**

Customize job behavior:

```go
// Configure retry behavior and priorities in your queue setup
```

### Send Emails

This project includes built-in email functionality with Mailpit for development testing.

**1. Create an email template**

Add your template in `email/`:

```go
// email/welcome.templ
package email

templ WelcomeEmail(userName string) {
    @BaseLayout() {
        <h1>Welcome, { userName }!</h1>
        <p>Thank you for joining us.</p>
    }
}
```

**2. Send the email**

```go
import (
    "deploycrate-ce/config"
    "deploycrate-ce/email"
)

// Send an email
data := email.TransactionalData{
    From:    config.DefaultSenderSignature,
    To:      []string{"user@example.com"},
    Subject: "Welcome!",
    Body:    WelcomeEmail("John Doe"),
}

err := email.SendTransactional(ctx, data, sender)
```

**3. Background email jobs**

For better performance, send emails asynchronously:

```go
// Enqueue email job through your queue
```

**Development Testing**

Emails are sent to Mailpit in development. Access the web UI at `http://localhost:8025` to view sent emails.

### Schema Changes

When modifying your database schema:

```bash
# 1. Create a migration
andurel migration new add_email_to_users

# 2. Edit the migration file
# Add your ALTER TABLE statements

# 3. Apply the migration
andurel migration up

# 4. Refresh affected models
andurel g model User --refresh
```
### Customize Styling

This project uses Tailwind CSS. Customize your theme in `css/themes.css`:

```css
@layer theme {
  :root {
    --color-primary: theme('colors.blue.600');
    --color-secondary: theme('colors.gray.600');
  }
}
```

Add reusable wrapper classes in `css/components.css` under `@layer components`, then import or compose in your views.
Use `examples/html/*.html` for snippet-based patterns (pure HTML + Datastar attributes).
The development server automatically rebuilds CSS on changes.

## Environment Configuration

Key environment variables (see `.env.example` for all options):

```bash
# Application
ENVIRONMENT=development
HOST=localhost
PORT=8080
PROJECT_NAME=deploycrate-ce
DOMAIN=localhost:8080
PROTOCOL=http

# Database
DB_KIND=postgres
DB_HOST=127.0.0.1
DB_PORT=5432
DB_NAME=deploycrate-ce_development
DB_USER=postgres
DB_PASSWORD=postgres
DB_SSL_MODE=disable

# Email (Mailpit for development)
MAILPIT_HOST=0.0.0.0
MAILPIT_PORT=1025
DEFAULT_SENDER_SIGNATURE=info@deploycrate-ce.com

# Security (auto-generated during scaffolding)
SESSION_KEY=<auto-generated>
SESSION_ENCRYPTION_KEY=<auto-generated>
SESSION_MAX_AGE=604800
TOKEN_SIGNING_KEY=<auto-generated>
PEPPER=<auto-generated>
PREVIOUS_PEPPERS=

# HTTP security
CORS_ALLOWED_ORIGINS=
CSRF_STRATEGY=header_only
CSRF_TRUSTED_ORIGINS=

# Telemetry (optional)
TELEMETRY_SERVICE_NAME=deploycrate-ce
TELEMETRY_SERVICE_VERSION=1.0.0
OTLP_LOGS_ENDPOINT=
OTLP_METRICS_ENDPOINT=
OTLP_TRACES_ENDPOINT=
TRACE_SAMPLE_RATE=1.0
```

## Session, CORS, and CSRF Protection

Application sessions use `HttpOnly` cookies with `SameSite=Lax` and `Path=/`. Production cookies also use `Secure`. `SESSION_MAX_AGE` is the lifetime in seconds and defaults to seven days (`604800`). Saving session state renews the expiration for another seven days. Signing out deletes the cookie immediately.

CORS allows credentials and trusts only the configured application origin (`PROTOCOL` + `DOMAIN`) by default. `CORS_ALLOWED_ORIGINS` accepts a comma-separated list of additional exact origins. Wildcard origins are rejected when the application starts.

CSRF protection uses Fetch Metadata to align with Rails behavior. Unsafe API requests bypass CSRF only when they carry a non-empty Bearer token and do not carry the application session cookie. Cookie-authenticated unsafe requests remain protected on every path.

**Strategies** (`CSRF_STRATEGY`):
- `header_only` (default): Unsafe requests must include the `Sec-Fetch-Site` header. Requests missing this header are rejected with `403`.
- `header_or_legacy_token`: Allows legacy form tokens when `Sec-Fetch-Site` is missing. Forms must submit `_csrf` or send `X-CSRF-Token` header.

**Trusted origins**:
- The base URL (`PROTOCOL` + `DOMAIN`) is always trusted automatically.
- `CSRF_TRUSTED_ORIGINS` accepts a comma-separated list of additional origins (e.g., `https://api.example.com,https://admin.example.com`).

**Client/testing tips**:
- For unsafe requests in tests or custom clients, include `Sec-Fetch-Site: same-origin`.
- When using `header_or_legacy_token`, submit `_csrf` with forms or send `X-CSRF-Token` header.

## Development Tips

1. **Live Reload**: Use `andurel run` during development for automatic reloading
2. **Type Safety**: Let Bun models and Templ catch errors at compile time
3. **Database Console**: Use `andurel app console` for quick database queries
4. **Hot Reload**: Changes to Go, Templ, or CSS automatically trigger rebuilds
5. **Tailwind**: Use Tailwind's utility classes in your Templ templates

## Common Tasks

```bash
# Start development
andurel run

# Create a new resource
andurel g resource Product

# Add a migration
andurel migration new add_field_to_products

# Run migrations
andurel migration up

# Access database console
andurel app console

# Run tests
go test ./...
```

## Integration Testing

This project includes a built-in integration testing framework that makes it easy to test controllers and models with real database interactions.

### Test Infrastructure

The framework provides:
- **Automatic test database setup**: Uses [testcontainers](https://golang.testcontainers.org/) to spin up PostgreSQL in Docker
- **Per-test databases**: Each test gets an isolated migrated database from a package-scoped PostgreSQL container
- **Factory pattern**: Simple builders for creating test data with sensible defaults

### Writing Controller Tests

**1. Create a test file** (e.g., `controllers/products_controller_test.go`):

```go
package controllers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/labstack/echo/v5"
	"deploycrate-ce/controllers"
	"deploycrate-ce/database"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"deploycrate-ce/models/factories"
)

var testCluster *storage.TestCluster

func TestMain(m *testing.M) {
	ctx := context.Background()
	var err error
	testCluster, err = storage.NewTestCluster(ctx)
	if err != nil {
		panic(err)
	}

	code := m.Run()
	if err := testCluster.Close(ctx); err != nil && code == 0 {
		panic(err)
	}
	os.Exit(code)
}

func TestProducts_Create(t *testing.T) {
	db := testCluster.NewTestDB(t, database.Migrations, "migrations")
	controller := controllers.NewProducts(db)

	// Create test request
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/products", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Test the controller action
	err := controller.Create(c)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Assert database state
	products, err := models.AllProducts(c.Request().Context(), db.Executor())
	if err != nil {
		t.Fatalf("failed to query products: %v", err)
	}

	if len(products) != 1 {
		t.Errorf("expected 1 product, got %d", len(products))
	}
}
```

### Creating Test Factories

**1. Create a factory** in `models/factories/product_factory.go`:

```go
package factories

import (
	"deploycrate-ce/models"
)

type ProductBuilder struct {
	data models.CreateProductData
}

func Product() *ProductBuilder {
	return &ProductBuilder{
		data: models.CreateProductData{
			Name:        "Test Product",
			Description: "Test description",
			Price:       "29.99",
		},
	}
}

func (b *ProductBuilder) WithName(name string) *ProductBuilder {
	b.data.Name = name
	return b
}

func (b *ProductBuilder) WithPrice(price string) *ProductBuilder {
	b.data.Price = price
	return b
}

func (b *ProductBuilder) Create(dbtx DBTX) models.Product {
	product, err := models.CreateProduct(ctx, dbtx, b.data)
	if err != nil {
		panic(err)
	}
	return product
}

func (b *ProductBuilder) Build() models.CreateProductData {
	return b.data
}
```

**2. Use factories in tests**:

```go
func TestProducts_Show(t *testing.T) {
	db := testCluster.NewTestDB(t, database.Migrations, "migrations")

	// Create test data with default values
	product := factories.Product().Create(db.Executor())

	// Or customize specific fields
	premiumProduct := factories.Product().
		WithName("Premium Product").
		WithPrice("99.99").
		Create(db.Executor())

	// Test your controller with the created data
	// ...
}
```

### Testing Patterns

**Test database queries**:

```go
func TestFindProduct(t *testing.T) {
	db := testCluster.NewTestDB(t, database.Migrations, "migrations")
	product := factories.Product().Create(db.Executor())

	found, err := models.FindProduct(context.Background(), db.Executor(), product.ID)
	if err != nil {
		t.Fatalf("FindProduct failed: %v", err)
	}

	if found.Name != product.Name {
		t.Errorf("expected name %s, got %s", product.Name, found.Name)
	}
}
```

**Test with multiple records**:

```go
func TestPaginateProducts(t *testing.T) {
	db := testCluster.NewTestDB(t, database.Migrations, "migrations")

	// Create test data
	for i := 0; i < 25; i++ {
		factories.Product().Create(db.Executor())
	}

	// Test pagination
	result, err := models.PaginateProducts(context.Background(), db.Executor(), 1, 10)
	if err != nil {
		t.Fatalf("PaginateProducts failed: %v", err)
	}

	if len(result.Products) != 10 {
		t.Errorf("expected 10 products, got %d", len(result.Products))
	}

	if result.TotalCount != 25 {
		t.Errorf("expected total count 25, got %d", result.TotalCount)
	}
}
```

**Test with related data**:

```go
func TestCreateOrder(t *testing.T) {
	db := testCluster.NewTestDB(t, database.Migrations, "migrations")

	// Create dependencies
	user := factories.User().Create(db.Executor())
	product := factories.Product().Create(db.Executor())

	// Test order creation
	order := factories.Order().
		WithUserID(user.ID).
		WithProductID(product.ID).
		Create(db.Executor())

	if order.UserID != user.ID {
		t.Errorf("order user_id mismatch")
	}
}
```

### Running Tests

```bash
# Run all tests
go test ./...

# Run tests in a specific package
go test ./controllers

# Run tests with coverage
go test -cover ./...

# Run a specific test
go test ./controllers -run TestProducts_Create

# Verbose output
go test -v ./...
```

### Test Database Setup

**Prerequisites**: Docker must be running to use testcontainers.

The test helper automatically:
1. Starts a PostgreSQL container with `postgres:17-alpine`
2. Creates an isolated database for each test
3. Runs embedded migrations from `database.Migrations`
4. Cleans up containers when tests complete

**Note**: The first test run will download the PostgreSQL Docker image, which may take a moment.

### Best Practices

1. **Use per-test databases**: Call `testCluster.NewTestDB(t, database.Migrations, "migrations")` in each test
2. **Use factories**: Create test data with factories instead of manual model creation
3. **Test isolation**: Each test should be independent and not rely on other tests
4. **Descriptive names**: Name tests clearly (e.g., `TestProducts_Create_WithInvalidData`)
5. **Assert clearly**: Check both success cases and expected database state
6. **Don't test frameworks**: Focus on your business logic, not Echo or Bun behavior

## Extensions

This project includes the following extensions:

### aws-ses

This project uses Amazon SES (Simple Email Service) for sending transactional and marketing emails in production.

#### Setup

**1. AWS Configuration**

You'll need an AWS account with SES configured:

```bash
# Verify your sender email address or domain in AWS SES Console
# https://console.aws.amazon.com/ses/

# Create IAM credentials with SES sending permissions
# Required policy: AmazonSESFullAccess or custom policy with ses:SendEmail
```

**2. Environment Variables**

Add these to your `.env` file:

```bash
# AWS SES Configuration
AWS_REGION=us-east-1                      # Your AWS region
AWS_SES_ACCESS_KEY_ID=your_access_key     # IAM access key
AWS_SES_SECRET_ACCESS_KEY=your_secret     # IAM secret key
AWS_SES_CONFIGURATION_SET=                # Optional: for open/click tracking
```

**3. Configuration Set (Optional)**

For email tracking (opens, clicks), create a Configuration Set in AWS SES:

1. Go to AWS SES Console → Configuration Sets
2. Create a new configuration set
3. Add event destinations (SNS, CloudWatch, Kinesis, etc.)
4. Set `AWS_SES_CONFIGURATION_SET` environment variable

#### Sending Transactional Emails

Transactional emails are one-to-one messages like password resets, order confirmations, and account notifications.

**Create an email template:**

```go
// email/order_confirmation.templ
package email

templ OrderConfirmation(orderID string, total string) {
    @BaseLayout() {
        <h1>Order Confirmed!</h1>
        <p>Thank you for your order #{orderID}.</p>
        <p>Total: {total}</p>
    }
}
```

**Send immediately (synchronous):**

```go
import "deploycrate-ce/email"

err := email.SendTransactional(ctx, email.TransactionalData{
    To:      "customer@example.com",
    Cc:      []string{"manager@example.com"},  // Optional
    From:    "orders@yourapp.com",
    Subject: "Order Confirmation",
    Component: email.OrderConfirmation("12345", "$99.99"),
    Attachments: []email.Attachment{  // Optional
        {
            Filename:    "invoice.pdf",
            ContentType: "application/pdf",
            Content:     pdfBytes,
        },
    },
}, emailClient)
```

**Send via background queue (recommended):**

```go
import "deploycrate-ce/queue/jobs"

_, err := insertOnly.Client.Insert(ctx, jobs.SendTransactionalEmailArgs{
    Data: email.TransactionalData{
        To:      "customer@example.com",
        From:    "orders@yourapp.com",
        Subject: "Order Confirmation",
        Component: email.OrderConfirmation("12345", "$99.99"),
    },
}, nil)
```

Benefits of queuing:
- Non-blocking: Returns immediately without waiting for AWS SES
- Automatic retries: Failed sends retry with exponential backoff
- Better reliability: Survives temporary AWS outages
- Observability: Track job status through River

#### Sending Marketing Emails

Marketing emails are bulk messages like newsletters, promotions, and announcements sent to multiple recipients.

**Important:** Marketing emails must include an unsubscribe link to comply with email regulations (CAN-SPAM, GDPR).

**Create a newsletter template:**

```go
// email/newsletter.templ
package email

templ Newsletter(recipientName string, unsubscribeURL string) {
    @BaseLayout() {
        <h1>Monthly Newsletter</h1>
        <p>Hi {recipientName},</p>
        <p>Here's what's new this month...</p>

        <footer>
            <a href={unsubscribeURL}>Unsubscribe</a>
        </footer>
    }
}
```

**Send to multiple recipients (queued pattern):**

```go
import "deploycrate-ce/queue/jobs"

// Queue individual emails for each recipient
recipients := []struct{
    Email string
    Name  string
    ID    string
}{
    {Email: "user1@example.com", Name: "Alice", ID: "user-123"},
    {Email: "user2@example.com", Name: "Bob", ID: "user-456"},
}

for _, recipient := range recipients {
    unsubscribeURL := fmt.Sprintf("https://yourapp.com/unsubscribe/%s", recipient.ID)

    _, err := insertOnly.Client.Insert(ctx, jobs.SendMarketingEmailArgs{
        Data: email.MarketingData{
            To:             []string{recipient.Email},
            From:           "newsletter@yourapp.com",
            Subject:        "Your Monthly Newsletter",
            Component:      email.Newsletter(recipient.Name, unsubscribeURL),
            UnsubscribeURL: unsubscribeURL,  // Required!
            Tags:           []string{"newsletter", "monthly"},
            TrackOpens:     true,   // Requires Configuration Set
            TrackClicks:    true,   // Requires Configuration Set
        },
    }, nil)

    if err != nil {
        // Handle error (log, retry, etc.)
        continue
    }
}
```

**Why queue individual emails?**

- **Personalization**: Each recipient gets customized content (name, preferences, etc.)
- **Tracking**: Individual delivery status and bounce tracking per recipient
- **Rate limiting**: AWS SES has sending limits; queuing prevents hitting them
- **Retries**: Failed emails retry automatically without affecting successful sends
- **Unsubscribe compliance**: Each email has a unique unsubscribe link

#### Email Tracking

Enable open and click tracking by configuring a Configuration Set in AWS SES:

```go
// Tracking is enabled per email
Data: email.MarketingData{
    // ... other fields
    TrackOpens:  true,
    TrackClicks: true,
}
```

**View tracking data:**
- Configure event destinations in your AWS SES Configuration Set
- Send events to CloudWatch, SNS, or Kinesis
- Build dashboards to visualize open/click rates

#### Testing in Development

During development, emails are sent to Mailpit instead of AWS SES:

```bash
# Mailpit configuration (automatically used in development)
MAILPIT_HOST=0.0.0.0
MAILPIT_PORT=1025
```

View test emails at `http://localhost:8025`

To test with real AWS SES in development:

1. Use the `aws-ses` extension
2. Set `ENVIRONMENT=production` or configure your app to use AWS SES in dev
3. Ensure your AWS credentials are valid

#### Error Handling

AWS SES provides detailed error information:

```go
err := email.SendTransactional(ctx, data, emailClient)
if err != nil {
    if email.IsValidationError(err) {
        // Invalid email address, missing required fields, etc.
        // Don't retry - fix the data
        log.Error("Invalid email data", "error", err)
    } else if email.IsTemporaryError(err) {
        // AWS throttling, service unavailable, etc.
        // Safe to retry
        log.Warn("Temporary email error, will retry", "error", err)
    } else if email.IsPermanentError(err) {
        // Account suspended, domain not verified, etc.
        // Don't retry - requires AWS console action
        log.Error("Permanent email error", "error", err)
    }
}
```

The background queue automatically handles retries for temporary errors and cancels jobs with validation or permanent errors.

#### AWS SES Limits

Be aware of AWS SES sending limits:

- **Sandbox**: 200 emails/day, verified recipients only
- **Production**: Request limit increase (up to millions/day)
- **Rate limit**: 14 emails/second (default, can be increased)

Request production access: AWS SES Console → Account Dashboard → Request Production Access

#### Best Practices

1. **Verify your domain**: Use domain verification instead of email verification for better deliverability
2. **Use Configuration Sets**: Enable tracking and monitoring
3. **Monitor bounce rates**: High bounce rates can hurt your sender reputation
4. **Handle suppression lists**: AWS SES automatically suppresses bounced/complained addresses
5. **Warm up your sending**: Gradually increase volume when starting with a new domain
6. **Use templates**: Create reusable email templates in AWS SES for simple use cases
7. **Queue emails**: Use background jobs for reliability and non-blocking sends

#### Resources

- [AWS SES Documentation](https://docs.aws.amazon.com/ses/)
- [AWS SES Pricing](https://aws.amazon.com/ses/pricing/) - $0.10 per 1,000 emails
- [SES Best Practices](https://docs.aws.amazon.com/ses/latest/dg/best-practices.html)
- [Moving out of Sandbox](https://docs.aws.amazon.com/ses/latest/dg/request-production-access.html)



## Learn More

- [Andurel Documentation](https://github.com/mbvlabs/andurel)
- [Echo Framework](https://echo.labstack.com/)
- [Templ](https://templ.guide/)
- [Datastar](https://data-star.dev/)
- [goqite](https://github.com/maragudk/goqite)
- [OpenTelemetry](https://opentelemetry.io/)

## Getting Help

For Andurel-specific questions and issues:
- GitHub Issues: https://github.com/mbvlabs/andurel/issues
- Documentation: https://github.com/mbvlabs/andurel

## License

DeployCrate Community Edition is licensed under the [GNU Affero General Public License v3.0 only](LICENSE) (`AGPL-3.0-only`). Unless a directory contains its own license, the AGPL applies to all code in this repository.

Standalone DeployCrate SDKs, API clients, and integration libraries may be released under the MIT or Apache 2.0 license and will identify that license in their own package. Proprietary DeployCrate Cloud features are not part of this repository.
