package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"deploycrate-ce/telemetry"

	"deploycrate-ce/internal/sudo"
)

const (
	dockerExecutable             = "/usr/bin/docker"
	registryAuthenticationRoot   = "/var/lib/deploycrate-builds"
	registryAuthenticationPrefix = "deploycrate-registry-auth-"
	registryAuthenticationOwner  = "deploycrate"
)

type Credentials struct {
	Endpoint string
	Username string
	Password string
}

type Authentication struct {
	DockerConfig string
}

func (authentication Authentication) Environment() []string {
	return []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=" + authentication.DockerConfig,
		"DOCKER_CONFIG=" + authentication.DockerConfig,
	}
}

func (authentication Authentication) Close() error {
	if authentication.DockerConfig == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return removeAuthenticationDirectory(ctx, authentication.DockerConfig)
}

type Client struct{}

func New() Client { return Client{} }

func (Client) Authenticate(ctx context.Context, credentials Credentials) (Authentication, error) {
	if strings.TrimSpace(credentials.Endpoint) == "" || strings.ContainsAny(credentials.Endpoint, " \t\r\n") ||
		strings.TrimSpace(credentials.Username) == "" || credentials.Password == "" {
		return Authentication{}, errors.New("registry endpoint and credentials are required")
	}
	directory, err := createAuthenticationDirectory(ctx)
	if err != nil {
		return Authentication{}, fmt.Errorf("create private Docker configuration: %w", err)
	}
	authentication := Authentication{DockerConfig: filepath.Clean(directory)}
	command := exec.CommandContext(ctx, dockerExecutable, "login", credentials.Endpoint, "--username", credentials.Username, "--password-stdin")
	command.Env = authentication.Environment()
	command.Stdin = bytes.NewBufferString(credentials.Password)
	output, err := command.CombinedOutput()
	if err != nil {
		_ = authentication.Close()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Authentication{}, fmt.Errorf("authenticate Docker registry client: %w", ctxErr)
		}
		return Authentication{}, fmt.Errorf("authenticate Docker registry client: %w: %s", err, sanitizedDockerError(output))
	}
	return authentication, nil
}

func createAuthenticationDirectory(ctx context.Context) (string, error) {
	output, err := sudo.CommandContext(
		ctx,
		"/usr/bin/mktemp",
		"-d",
		"-p",
		registryAuthenticationRoot,
		registryAuthenticationPrefix+"XXXXXXXXXX",
	).CombinedOutput()
	if err != nil {
		return "", commandError(err, output)
	}
	directory := filepath.Clean(strings.TrimSpace(string(output)))
	if !validAuthenticationDirectory(directory) {
		return "", errors.New("privileged Docker configuration path is invalid")
	}
	if output, err := sudo.CommandContext(
		ctx,
		"/usr/bin/chown",
		registryAuthenticationOwner+":"+registryAuthenticationOwner,
		"--",
		directory,
	).CombinedOutput(); err != nil {
		cleanupAuthenticationDirectory(ctx, directory)
		return "", commandError(err, output)
	}
	if output, err := sudo.CommandContext(ctx, "/usr/bin/chmod", "0700", "--", directory).CombinedOutput(); err != nil {
		cleanupAuthenticationDirectory(ctx, directory)
		return "", commandError(err, output)
	}
	return directory, nil
}

func cleanupAuthenticationDirectory(ctx context.Context, directory string) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	_ = removeAuthenticationDirectory(cleanupCtx, directory)
}

func removeAuthenticationDirectory(ctx context.Context, directory string) error {
	directory = filepath.Clean(directory)
	if !validAuthenticationDirectory(directory) {
		return errors.New("Docker configuration cleanup path is invalid")
	}
	output, err := sudo.CommandContext(ctx, "/usr/bin/rm", "-rf", "--", directory).CombinedOutput()
	if err != nil {
		return commandError(err, output)
	}
	return nil
}

func validAuthenticationDirectory(directory string) bool {
	base := filepath.Base(directory)
	return filepath.Dir(directory) == registryAuthenticationRoot &&
		strings.HasPrefix(base, registryAuthenticationPrefix) &&
		len(base) > len(registryAuthenticationPrefix)
}

func commandError(commandErr error, output []byte) error {
	message := strings.TrimSpace(string(output))
	if message == "" {
		return commandErr
	}
	return errors.New(message)
}

func (Client) Pull(ctx context.Context, authentication Authentication, image string) error {
	if !strings.Contains(image, "@sha256:") {
		return errors.New("registry pull requires an immutable image digest")
	}
	command := exec.CommandContext(ctx, dockerExecutable, "pull", image)
	command.Env = authentication.Environment()
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("pull immutable registry image: %w: %s", err, sanitizedDockerError(output))
	}
	return nil
}

func (Client) ResolveDigest(ctx context.Context, authentication Authentication, imageTag string) (string, error) {
	command := exec.CommandContext(ctx, dockerExecutable, "image", "inspect", "--format", "{{json .RepoDigests}}", imageTag)
	command.Env = authentication.Environment()
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("inspect published image digest: %w: %s", err, sanitizedDockerError(output))
	}
	var references []string
	if err := json.Unmarshal(bytes.TrimSpace(output), &references); err != nil {
		return "", errors.New("published image digest response is invalid")
	}
	for _, reference := range references {
		if strings.Contains(reference, "@sha256:") {
			return reference, nil
		}
	}
	return "", errors.New("published image has no immutable registry digest")
}

func (Client) ResolveRemoteDigest(ctx context.Context, credentials Credentials, imageTag string) (string, error) {
	endpoint := strings.TrimSuffix(strings.TrimSpace(credentials.Endpoint), "/")
	imageTag = strings.TrimSpace(imageTag)
	prefix := endpoint + "/"
	if !strings.HasPrefix(imageTag, prefix) || strings.Contains(imageTag, "@") {
		return "", errors.New("published image must belong to the authenticated registry and use a tag")
	}
	repositoryAndTag := strings.TrimPrefix(imageTag, prefix)
	separator := strings.LastIndex(repositoryAndTag, ":")
	if separator <= 0 || separator == len(repositoryAndTag)-1 {
		return "", errors.New("published image tag is invalid")
	}
	repository, tag := repositoryAndTag[:separator], repositoryAndTag[separator+1:]
	apiEndpoint := endpoint
	if endpoint == "docker.io" {
		apiEndpoint = "registry-1.docker.io"
	}
	requestURL := "https://" + apiEndpoint + "/v2/" + repository + "/manifests/" + url.PathEscape(tag)
	accept := strings.Join([]string{
		"application/vnd.oci.image.index.v1+json",
		"application/vnd.docker.distribution.manifest.list.v2+json",
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
	}, ", ")
	client := telemetry.NewHTTPClient(30 * time.Second)
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errors.New("registry manifest inspection does not allow redirects")
	}
	response, err := manifestHead(ctx, client, requestURL, accept, credentials, "")
	if err != nil {
		return "", fmt.Errorf("inspect published registry manifest: %w", err)
	}
	if response.StatusCode == http.StatusUnauthorized {
		challenge := response.Header.Get("WWW-Authenticate")
		_ = response.Body.Close()
		token, tokenErr := registryBearerToken(ctx, client, challenge, credentials)
		if tokenErr != nil {
			return "", fmt.Errorf("authenticate published registry manifest inspection: %w", tokenErr)
		}
		response, err = manifestHead(ctx, client, requestURL, accept, credentials, token)
		if err != nil {
			return "", fmt.Errorf("inspect published registry manifest: %w", err)
		}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("inspect published registry manifest: status %d", response.StatusCode)
	}
	digest := strings.ToLower(strings.TrimSpace(response.Header.Get("Docker-Content-Digest")))
	if len(digest) != 71 || !strings.HasPrefix(digest, "sha256:") {
		return "", errors.New("registry did not return a valid immutable image digest")
	}
	for _, character := range strings.TrimPrefix(digest, "sha256:") {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return "", errors.New("registry returned an invalid immutable image digest")
		}
	}
	return endpoint + "/" + repository + "@" + digest, nil
}

func manifestHead(
	ctx context.Context,
	client *http.Client,
	requestURL, accept string,
	credentials Credentials,
	bearerToken string,
) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, requestURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", accept)
	if bearerToken != "" {
		request.Header.Set("Authorization", "Bearer "+bearerToken)
	} else {
		request.SetBasicAuth(credentials.Username, credentials.Password)
	}
	return client.Do(request)
}

func registryBearerToken(
	ctx context.Context,
	client *http.Client,
	challenge string,
	credentials Credentials,
) (string, error) {
	scheme, parametersText, found := strings.Cut(strings.TrimSpace(challenge), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", errors.New("registry did not provide a supported authentication challenge")
	}
	parameters := authenticationChallengeParameters(parametersText)
	realm := strings.TrimSpace(parameters["realm"])
	if realm == "" {
		return "", errors.New("registry authentication challenge has no realm")
	}
	tokenURL, err := url.Parse(realm)
	if err != nil || tokenURL.Scheme != "https" || tokenURL.Host == "" {
		return "", errors.New("registry authentication realm is invalid")
	}
	query := tokenURL.Query()
	for _, key := range []string{"service", "scope"} {
		if value := strings.TrimSpace(parameters[key]); value != "" {
			query.Set(key, value)
		}
	}
	tokenURL.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL.String(), nil)
	if err != nil {
		return "", err
	}
	request.SetBasicAuth(credentials.Username, credentials.Password)
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("registry token service returned status %d", response.StatusCode)
	}
	var payload struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if json.NewDecoder(response.Body).Decode(&payload) != nil {
		return "", errors.New("registry token response is invalid")
	}
	if strings.TrimSpace(payload.Token) != "" {
		return payload.Token, nil
	}
	if strings.TrimSpace(payload.AccessToken) != "" {
		return payload.AccessToken, nil
	}
	return "", errors.New("registry token response has no token")
}

func authenticationChallengeParameters(value string) map[string]string {
	parameters := make(map[string]string)
	for len(value) > 0 {
		value = strings.TrimLeft(value, " ,\t")
		equals := strings.IndexByte(value, '=')
		if equals <= 0 {
			break
		}
		key := strings.ToLower(strings.TrimSpace(value[:equals]))
		value = strings.TrimLeft(value[equals+1:], " \t")
		if !strings.HasPrefix(value, "\"") {
			comma := strings.IndexByte(value, ',')
			if comma < 0 {
				parameters[key] = strings.TrimSpace(value)
				break
			}
			parameters[key] = strings.TrimSpace(value[:comma])
			value = value[comma+1:]
			continue
		}
		value = value[1:]
		end := strings.IndexByte(value, '"')
		if end < 0 {
			break
		}
		parameters[key] = value[:end]
		value = value[end+1:]
	}
	return parameters
}

func sanitizedDockerError(output []byte) string {
	message := strings.TrimSpace(string(output))
	if len(message) > 2048 {
		message = message[:2048]
	}
	return message
}
