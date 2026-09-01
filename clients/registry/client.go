package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"sort"
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
	registryListPageSize         = 100
	registryListLimit            = 10000
	registryListResponseLimit    = 4 << 20
)

type Credentials struct {
	Endpoint string
	Username string
	Password string
}

type Authentication struct {
	DockerConfig string
}

type Repository struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
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
	if strings.TrimSpace(credentials.Endpoint) == "" ||
		strings.ContainsAny(credentials.Endpoint, " \t\r\n") ||
		strings.TrimSpace(credentials.Username) == "" ||
		credentials.Password == "" {
		return Authentication{}, errors.New("registry endpoint and credentials are required")
	}
	directory, err := createAuthenticationDirectory(ctx)
	if err != nil {
		return Authentication{}, fmt.Errorf("create private Docker configuration: %w", err)
	}
	authentication := Authentication{DockerConfig: filepath.Clean(directory)}
	command := exec.CommandContext(
		ctx,
		dockerExecutable,
		"login",
		credentials.Endpoint,
		"--username",
		credentials.Username,
		"--password-stdin",
	)
	command.Env = authentication.Environment()
	command.Stdin = bytes.NewBufferString(credentials.Password)
	output, err := command.CombinedOutput()
	if err != nil {
		_ = authentication.Close()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Authentication{}, fmt.Errorf("authenticate Docker registry client: %w", ctxErr)
		}
		return Authentication{}, fmt.Errorf(
			"authenticate Docker registry client: %w: %s",
			err,
			sanitizedDockerError(output),
		)
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
	if output, err := sudo.CommandContext(ctx, "/usr/bin/chmod", "0700", "--", directory).
		CombinedOutput(); err != nil {
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
		return fmt.Errorf(
			"pull immutable registry image: %w: %s",
			err,
			sanitizedDockerError(output),
		)
	}
	return nil
}

func (Client) ResolveDigest(
	ctx context.Context,
	authentication Authentication,
	imageTag string,
) (string, error) {
	command := exec.CommandContext(
		ctx,
		dockerExecutable,
		"image",
		"inspect",
		"--format",
		"{{json .RepoDigests}}",
		imageTag,
	)
	command.Env = authentication.Environment()
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf(
			"inspect published image digest: %w: %s",
			err,
			sanitizedDockerError(output),
		)
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

func (Client) ResolveRemoteDigest(
	ctx context.Context,
	credentials Credentials,
	imageReference string,
) (string, error) {
	endpoint := strings.TrimSuffix(strings.TrimSpace(credentials.Endpoint), "/")
	imageReference = strings.TrimSpace(imageReference)
	prefix := endpoint + "/"
	if !strings.HasPrefix(imageReference, prefix) {
		return "", errors.New("image must belong to the authenticated registry")
	}
	repositoryAndReference := strings.TrimPrefix(imageReference, prefix)
	repository := ""
	manifestReference := ""
	requestedDigest := ""
	if before, after, found := strings.Cut(repositoryAndReference, "@"); found {
		repository, manifestReference, requestedDigest = before, after, strings.ToLower(after)
		if !validSHA256Digest(requestedDigest) {
			return "", errors.New("image digest is invalid")
		}
	} else {
		separator := strings.LastIndex(repositoryAndReference, ":")
		if separator <= 0 || separator == len(repositoryAndReference)-1 {
			return "", errors.New("image tag is invalid")
		}
		repository, manifestReference = repositoryAndReference[:separator], repositoryAndReference[separator+1:]
	}
	if strings.Trim(repository, "/") != repository || strings.ContainsAny(repository, " \\?#") {
		return "", errors.New("image repository is invalid")
	}
	apiRepository := repository
	apiEndpoint := endpoint
	if endpoint == "docker.io" {
		apiEndpoint = "registry-1.docker.io"
		if !strings.Contains(apiRepository, "/") {
			apiRepository = "library/" + apiRepository
		}
	}
	requestURL := "https://" + apiEndpoint + "/v2/" + apiRepository + "/manifests/" + url.PathEscape(
		manifestReference,
	)
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
		token, tokenErr := registryBearerToken(ctx, client, challenge, credentials, "")
		if tokenErr != nil {
			return "", fmt.Errorf(
				"authenticate published registry manifest inspection: %w",
				tokenErr,
			)
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
	if !validSHA256Digest(digest) {
		return "", errors.New("registry did not return a valid immutable image digest")
	}
	if requestedDigest != "" && digest != requestedDigest {
		return "", errors.New("registry returned a different digest than requested")
	}
	return endpoint + "/" + repository + "@" + digest, nil
}

func (Client) DeleteManifest(ctx context.Context, credentials Credentials, reference string) error {
	endpoint := strings.TrimSuffix(strings.TrimSpace(credentials.Endpoint), "/")
	reference = strings.TrimSpace(reference)
	prefix := endpoint + "/"
	if !strings.HasPrefix(reference, prefix) {
		return errors.New("image must belong to the authenticated registry")
	}
	repositoryAndReference := strings.TrimPrefix(reference, prefix)
	repository, manifestReference, found := strings.Cut(repositoryAndReference, "@")
	if !found {
		return errors.New("image deletion requires an immutable digest")
	}
	digest := strings.ToLower(manifestReference)
	if !validSHA256Digest(digest) {
		return errors.New("image digest is invalid")
	}
	if strings.Trim(repository, "/") != repository ||
		strings.ContainsAny(repository, " \\?#\t\r\n") {
		return errors.New("image repository is invalid")
	}
	apiRepository := repository
	apiEndpoint := endpoint
	if endpoint == "docker.io" {
		apiEndpoint = "registry-1.docker.io"
		if !strings.Contains(apiRepository, "/") {
			apiRepository = "library/" + apiRepository
		}
	}
	segments := strings.Split(apiRepository, "/")
	for index := range segments {
		segments[index] = url.PathEscape(segments[index])
	}
	requestURL := "https://" + apiEndpoint + "/v2/" + strings.Join(
		segments,
		"/",
	) + "/manifests/" + digest
	client := telemetry.NewHTTPClient(30 * time.Second)
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errors.New("registry image deletion does not allow redirects")
	}
	request, err := registryDeleteManifestRequest(ctx, requestURL, credentials, "")
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("delete registry image: %w", err)
	}
	if response.StatusCode == http.StatusUnauthorized {
		challenge := response.Header.Get("WWW-Authenticate")
		_ = response.Body.Close()
		token, tokenErr := registryBearerToken(
			ctx,
			client,
			challenge,
			credentials,
			"repository:"+apiRepository+":pull,delete",
		)
		if tokenErr != nil {
			return fmt.Errorf("authenticate registry image deletion: %w", tokenErr)
		}
		request, err = registryDeleteManifestRequest(ctx, requestURL, credentials, token)
		if err != nil {
			return err
		}
		response, err = client.Do(request)
		if err != nil {
			return fmt.Errorf("delete registry image: %w", err)
		}
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK, http.StatusAccepted, http.StatusNotFound:
		return nil
	case http.StatusMethodNotAllowed:
		return errors.New("registry does not allow deleting images")
	case http.StatusUnauthorized, http.StatusForbidden:
		return errors.New("registry does not allow deleting this image")
	default:
		return fmt.Errorf("delete registry image: status %d", response.StatusCode)
	}
}

func registryDeleteManifestRequest(
	ctx context.Context,
	requestURL string,
	credentials Credentials,
	bearerToken string,
) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodDelete, requestURL, nil)
	if err != nil {
		return nil, err
	}
	if bearerToken != "" {
		request.Header.Set("Authorization", "Bearer "+bearerToken)
	} else {
		request.SetBasicAuth(credentials.Username, credentials.Password)
	}
	return request, nil
}

func (Client) ListRepositories(ctx context.Context, credentials Credentials) ([]Repository, error) {
	endpoint := strings.TrimSuffix(strings.TrimSpace(credentials.Endpoint), "/")
	if endpoint == "" || strings.ContainsAny(endpoint, "/ \\?#\t\r\n") ||
		strings.TrimSpace(credentials.Username) == "" || credentials.Password == "" {
		return nil, errors.New("registry endpoint and credentials are required")
	}

	client := telemetry.NewHTTPClient(30 * time.Second)
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errors.New("registry inventory does not allow redirects")
	}
	catalogURL := &url.URL{Scheme: "https", Host: endpoint, Path: "/v2/_catalog"}
	query := catalogURL.Query()
	query.Set("n", fmt.Sprintf("%d", registryListPageSize))
	catalogURL.RawQuery = query.Encode()

	names := make([]string, 0)
	next := catalogURL.String()
	seenPages := make(map[string]struct{})
	for next != "" {
		if _, seen := seenPages[next]; seen {
			return nil, errors.New("Registry repository pagination repeated a page")
		}
		seenPages[next] = struct{}{}
		var payload struct {
			Repositories []string `json:"repositories"`
		}
		page, err := registryJSONPage(ctx, client, next, credentials, &payload)
		if err != nil {
			return nil, fmt.Errorf("list Registry repositories: %w", err)
		}
		for _, name := range payload.Repositories {
			name = strings.TrimSpace(name)
			if name == "" || len(name) > 512 || strings.ContainsAny(name, " \\?#\t\r\n") ||
				strings.Trim(name, "/") != name {
				return nil, errors.New("Registry returned an invalid repository name")
			}
			names = append(names, name)
			if len(names) > registryListLimit {
				return nil, errors.New("Registry repository inventory exceeds the supported limit")
			}
		}
		next, err = nextRegistryPage(page.URL, page.Header.Get("Link"))
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(names)

	repositories := make([]Repository, 0, len(names))
	for _, name := range names {
		tags, err := listRepositoryTags(ctx, client, endpoint, name, credentials)
		if err != nil {
			return nil, fmt.Errorf("list Registry repository %s tags: %w", name, err)
		}
		repositories = append(repositories, Repository{Name: name, Tags: tags})
	}
	return repositories, nil
}

func listRepositoryTags(
	ctx context.Context,
	client *http.Client,
	endpoint, repository string,
	credentials Credentials,
) ([]string, error) {
	segments := strings.Split(repository, "/")
	for index := range segments {
		segments[index] = url.PathEscape(segments[index])
	}
	tagsURL := &url.URL{
		Scheme: "https",
		Host:   endpoint,
		Path:   "/v2/" + strings.Join(segments, "/") + "/tags/list",
	}
	query := tagsURL.Query()
	query.Set("n", fmt.Sprintf("%d", registryListPageSize))
	tagsURL.RawQuery = query.Encode()

	tags := make([]string, 0)
	next := tagsURL.String()
	seenPages := make(map[string]struct{})
	for next != "" {
		if _, seen := seenPages[next]; seen {
			return nil, errors.New("Registry tag pagination repeated a page")
		}
		seenPages[next] = struct{}{}
		var payload struct {
			Tags []string `json:"tags"`
		}
		page, err := registryJSONPage(ctx, client, next, credentials, &payload)
		if err != nil {
			return nil, err
		}
		for _, tag := range payload.Tags {
			tag = strings.TrimSpace(tag)
			if tag == "" || len(tag) > 128 || strings.ContainsAny(tag, "/ \\?#\t\r\n") {
				return nil, errors.New("Registry returned an invalid image tag")
			}
			tags = append(tags, tag)
			if len(tags) > registryListLimit {
				return nil, errors.New("Registry tag inventory exceeds the supported limit")
			}
		}
		next, err = nextRegistryPage(page.URL, page.Header.Get("Link"))
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(tags)
	return tags, nil
}

type registryPage struct {
	URL    *url.URL
	Header http.Header
}

func registryJSONPage(
	ctx context.Context,
	client *http.Client,
	requestURL string,
	credentials Credentials,
	target any,
) (registryPage, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return registryPage{}, err
	}
	request.Header.Set("Accept", "application/json")
	request.SetBasicAuth(credentials.Username, credentials.Password)
	response, err := client.Do(request)
	if err != nil {
		return registryPage{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return registryPage{}, fmt.Errorf(
			"Registry inventory returned status %d",
			response.StatusCode,
		)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, registryListResponseLimit)).
		Decode(target); err != nil {
		return registryPage{}, errors.New("Registry inventory response is invalid")
	}
	return registryPage{URL: response.Request.URL, Header: response.Header.Clone()}, nil
}

func nextRegistryPage(current *url.URL, header string) (string, error) {
	for part := range strings.SplitSeq(header, ",") {
		part = strings.TrimSpace(part)
		if !strings.Contains(strings.ToLower(part), `rel="next"`) {
			continue
		}
		start := strings.IndexByte(part, '<')
		end := strings.IndexByte(part, '>')
		if start < 0 || end <= start+1 {
			return "", errors.New("Registry inventory pagination link is invalid")
		}
		reference, err := url.Parse(part[start+1 : end])
		if err != nil {
			return "", errors.New("Registry inventory pagination link is invalid")
		}
		next := current.ResolveReference(reference)
		if next.Scheme != current.Scheme || !strings.EqualFold(next.Host, current.Host) ||
			!strings.HasPrefix(next.Path, "/v2/") {
			return "", errors.New("Registry inventory pagination changed origin")
		}
		return next.String(), nil
	}
	return "", nil
}

func validSHA256Digest(value string) bool {
	if len(value) != 71 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	for _, character := range strings.TrimPrefix(value, "sha256:") {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
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
	requestedScope string,
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
	if requestedScope != "" {
		query.Set("scope", requestedScope)
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
