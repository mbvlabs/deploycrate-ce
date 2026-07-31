package resourceaccess

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

type nativeDatabaseInstallSpec struct {
	InstallationID        string          `json:"installationId"`
	Engine                string          `json:"engine"`
	EngineVersion         string          `json:"engineVersion"`
	PackageName           string          `json:"packageName"`
	PackageVersion        string          `json:"packageVersion"`
	ServiceName           string          `json:"serviceName"`
	ConfigPath            string          `json:"configPath"`
	DataPath              string          `json:"dataPath"`
	Port                  int32           `json:"port"`
	AdministratorUsername string          `json:"administratorUsername"`
	AdministratorPassword string          `json:"administratorPassword"`
	Settings              json.RawMessage `json:"settings"`
}

type nativeDatabaseState struct {
	Installed        bool   `json:"installed"`
	InstalledVersion string `json:"installedVersion"`
	ServiceState     string `json:"serviceState"`
	Running          bool   `json:"running"`
	Error            string `json:"error"`
}

func installNativeDatabase(input io.Reader) error {
	decoder := json.NewDecoder(io.LimitReader(input, 1024*1024))
	decoder.DisallowUnknownFields()
	var spec nativeDatabaseInstallSpec
	if err := decoder.Decode(&spec); err != nil {
		return fmt.Errorf("decode native database installation: %w", err)
	}
	defer func() { spec.AdministratorPassword = "" }()
	if err := validateNativeDatabaseInstallSpec(spec); err != nil {
		return err
	}
	packageReference := spec.PackageName
	if spec.PackageVersion != "" {
		packageReference += "=" + spec.PackageVersion
	}
	if err := run("/usr/bin/apt-get", "install", "--yes", "--no-install-recommends", packageReference); err != nil {
		return err
	}
	if err := runAllowInactive("/usr/bin/systemctl", "stop", spec.ServiceName); err != nil {
		return err
	}
	if err := run("/usr/bin/install", "-d", "-m", "0700", "-o", "postgres", "-g", "postgres", spec.DataPath); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(spec.DataPath, "PG_VERSION")); errors.Is(err, os.ErrNotExist) {
		initDB := filepath.Join("/usr/lib/postgresql", spec.EngineVersion, "bin/initdb")
		if err := run("/usr/sbin/runuser", "--user", "postgres", "--", initDB, "--pgdata", spec.DataPath, "--auth-local=peer", "--auth-host=scram-sha-256"); err != nil {
			return err
		}
	} else if err != nil {
		return fmt.Errorf("inspect native PostgreSQL data directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(spec.ConfigPath), 0o755); err != nil {
		return fmt.Errorf("create native database configuration directory: %w", err)
	}
	configuration := []byte("# Managed by DeployCrate\n" +
		"data_directory = '" + spec.DataPath + "'\n" +
		"listen_addresses = '127.0.0.1'\n" +
		"port = " + strconv.Itoa(int(spec.Port)) + "\n" +
		"password_encryption = 'scram-sha-256'\n")
	if err := os.WriteFile(spec.ConfigPath, configuration, 0o640); err != nil {
		return fmt.Errorf("write native database configuration: %w", err)
	}
	if err := run("/usr/bin/chown", "root:postgres", spec.ConfigPath); err != nil {
		return err
	}
	if err := run("/usr/bin/systemctl", "enable", spec.ServiceName); err != nil {
		return err
	}
	if err := run("/usr/bin/systemctl", "restart", spec.ServiceName); err != nil {
		return err
	}
	return initializePostgreSQLAdministrator(spec)
}

func validateNativeDatabaseInstallSpec(spec nativeDatabaseInstallSpec) error {
	if _, err := uuid.Parse(spec.InstallationID); err != nil {
		return errors.New("native database installation ID must be a UUID")
	}
	if spec.Engine != "postgresql" {
		return errors.New("native database installations currently support PostgreSQL only")
	}
	if !validPostgreSQLMajorVersion(spec.EngineVersion) {
		return errors.New("native PostgreSQL engine version must be a major version number")
	}
	if !validNativeToken(spec.PackageName) || !validNativeToken(spec.ServiceName) {
		return errors.New("native database package and service names are invalid")
	}
	if spec.PackageVersion != "" && !validNativeVersion(spec.PackageVersion) {
		return errors.New("native database package version is invalid")
	}
	if !filepath.IsAbs(spec.ConfigPath) || !filepath.IsAbs(spec.DataPath) || !validNativeDatabasePath(spec.ConfigPath) || !validNativeDatabasePath(spec.DataPath) || !json.Valid(spec.Settings) {
		return errors.New("native database paths and settings are invalid")
	}
	expectedConfigurationDirectory := filepath.Join("/etc/postgresql", spec.EngineVersion, "main/conf.d") + string(filepath.Separator)
	cleanConfigPath := filepath.Clean(spec.ConfigPath)
	if !strings.HasPrefix(cleanConfigPath, expectedConfigurationDirectory) || filepath.Ext(cleanConfigPath) != ".conf" ||
		!strings.HasPrefix(filepath.Clean(spec.DataPath), "/var/lib/") {
		return errors.New("native PostgreSQL configuration must be a .conf file in its loaded conf.d directory and data must stay under /var/lib")
	}
	if spec.Port < 1 || spec.Port > 65535 {
		return errors.New("native PostgreSQL port is invalid")
	}
	if strings.TrimSpace(spec.AdministratorUsername) == "" || len([]byte(spec.AdministratorUsername)) > 63 || strings.ContainsRune(spec.AdministratorUsername, '\x00') {
		return errors.New("native PostgreSQL administrator username is invalid")
	}
	if spec.AdministratorPassword == "" || len([]byte(spec.AdministratorPassword)) > 4096 || strings.ContainsRune(spec.AdministratorPassword, '\x00') {
		return errors.New("native PostgreSQL administrator password is invalid")
	}
	return nil
}

func validPostgreSQLMajorVersion(value string) bool {
	if value == "" || len(value) > 3 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func initializePostgreSQLAdministrator(spec nativeDatabaseInstallSpec) error {
	username := base64.StdEncoding.EncodeToString([]byte(spec.AdministratorUsername))
	password := base64.StdEncoding.EncodeToString([]byte(spec.AdministratorPassword))
	script := fmt.Appendf(nil, `\set ON_ERROR_STOP on
SELECT format(
  'CREATE ROLE %%I WITH LOGIN SUPERUSER CREATEDB CREATEROLE',
  convert_from(decode('%s', 'base64'), 'UTF8')
)
WHERE NOT EXISTS (
  SELECT 1 FROM pg_catalog.pg_roles
  WHERE rolname = convert_from(decode('%s', 'base64'), 'UTF8')
) \gexec
SELECT format(
  'ALTER ROLE %%I WITH LOGIN SUPERUSER CREATEDB CREATEROLE PASSWORD %%L',
  convert_from(decode('%s', 'base64'), 'UTF8'),
  convert_from(decode('%s', 'base64'), 'UTF8')
) \gexec
`, username, username, username, password)
	defer clear(script)
	command := exec.Command("/usr/sbin/runuser", "--user", "postgres", "--", "/usr/bin/psql", "--no-psqlrc", "--port", strconv.Itoa(int(spec.Port)), "--dbname", "postgres")
	command.Stdin = bytes.NewReader(script)
	_, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("initialize native PostgreSQL administrator: %w", err)
	}
	return nil
}

func validNativeDatabasePath(value string) bool {
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') &&
			(character < '0' || character > '9') && character != '/' && character != '-' && character != '_' && character != '.' {
			return false
		}
	}
	return value != ""
}

func printNativeDatabaseInspection(installationID, packageName, serviceName string) error {
	if _, err := uuid.Parse(installationID); err != nil || !validNativeToken(packageName) || !validNativeToken(serviceName) {
		return errors.New("native database inspection identity is invalid")
	}
	state := nativeDatabaseState{}
	if output, err := exec.Command("/usr/bin/dpkg-query", "--show", "--showformat=${Version}", packageName).Output(); err == nil {
		state.Installed = true
		state.InstalledVersion = strings.TrimSpace(string(output))
	}
	output, err := exec.Command("/usr/bin/systemctl", "is-active", serviceName).CombinedOutput()
	state.ServiceState = strings.TrimSpace(string(output))
	state.Running = err == nil && state.ServiceState == "active"
	if err != nil && state.ServiceState != "inactive" && state.ServiceState != "failed" && state.ServiceState != "unknown" {
		state.Error = "system service state could not be determined"
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}

func controlNativeDatabase(operation, installationID, serviceName string) error {
	if _, err := uuid.Parse(installationID); err != nil || !validNativeToken(serviceName) {
		return errors.New("native database service identity is invalid")
	}
	if operation != "start" && operation != "stop" && operation != "restart" {
		return errors.New("native database service operation is unsupported")
	}
	return run("/usr/bin/systemctl", operation, serviceName)
}

func validNativeToken(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') &&
			(character < '0' || character > '9') && character != '-' && character != '_' && character != '.' && character != '@' {
			return false
		}
	}
	return true
}

func validNativeVersion(value string) bool {
	if len(value) > 128 {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') &&
			(character < '0' || character > '9') && character != '-' && character != '_' && character != '.' && character != '+' && character != ':' && character != '~' {
			return false
		}
	}
	return true
}
