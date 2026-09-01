package github

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"deploycrate-ce/telemetry"
)

const (
	APIVersion      = "2026-03-10"
	defaultBaseURL  = "https://api.github.com"
	maxResponseBody = 2 << 20
	maxArchiveBytes = 512 << 20
)

var (
	ErrUnauthorized = errors.New("GitHub rejected the App credentials")
	ErrNotFound     = errors.New("GitHub resource not found")
	ErrRateLimited  = errors.New("GitHub API rate limit exceeded")
)

type Client struct {
	httpClient *http.Client
	baseURL    string
}

func (c *Client) DownloadArchive(
	ctx context.Context,
	auth AppAuthentication,
	installationID int64,
	repository, revision string,
	destination io.Writer,
) error {
	repository = strings.Trim(strings.TrimSpace(repository), "/")
	revision = strings.ToLower(strings.TrimSpace(revision))
	if installationID <= 0 || strings.Count(repository, "/") != 1 || len(revision) != 40 ||
		destination == nil {
		return errors.New("GitHub repository, exact revision, and archive destination are required")
	}
	for _, character := range revision {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return errors.New("GitHub archive revision must be an exact commit SHA")
		}
	}
	token, err := c.createInstallationToken(ctx, auth, installationID)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/repos/"+repository+"/tarball/"+revision, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", APIVersion)
	request.Header.Set("User-Agent", "DeployCrate-CE")
	request.Header.Set("Authorization", "Bearer "+token)
	client := *c.httpClient
	client.Timeout = 2 * time.Minute
	client.CheckRedirect = func(next *http.Request, previous []*http.Request) error {
		if len(previous) > 3 {
			return errors.New("GitHub archive exceeded the redirect limit")
		}
		host := strings.ToLower(next.URL.Hostname())
		if host != "api.github.com" && host != "github.com" && host != "codeload.github.com" {
			return errors.New("GitHub archive redirected to an untrusted host")
		}
		if host != "api.github.com" {
			next.Header.Del("Authorization")
		}
		return nil
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("download GitHub archive: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if response.StatusCode == http.StatusNotFound {
			return ErrNotFound
		}
		if response.StatusCode == http.StatusUnauthorized ||
			response.StatusCode == http.StatusForbidden {
			return ErrUnauthorized
		}
		return fmt.Errorf("GitHub archive returned status %d", response.StatusCode)
	}
	limited := &io.LimitedReader{R: response.Body, N: maxArchiveBytes + 1}
	written, err := io.Copy(destination, limited)
	if err != nil {
		return fmt.Errorf("write GitHub archive: %w", err)
	}
	if written > maxArchiveBytes {
		return errors.New("GitHub archive exceeded the allowed size")
	}
	return nil
}

func NewClient() *Client {
	return &Client{httpClient: telemetry.NewHTTPClient(20 * time.Second), baseURL: defaultBaseURL}
}

type Account struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
	Type  string `json:"type"`
}

type ManifestApp struct {
	ID            int64             `json:"id"`
	ClientID      string            `json:"client_id"`
	ClientSecret  string            `json:"client_secret"`
	WebhookSecret string            `json:"webhook_secret"`
	PEM           string            `json:"pem"`
	Slug          string            `json:"slug"`
	Name          string            `json:"name"`
	HTMLURL       string            `json:"html_url"`
	Owner         Account           `json:"owner"`
	Permissions   map[string]string `json:"permissions"`
	Events        []string          `json:"events"`
}

type Installation struct {
	ID                  int64             `json:"id"`
	AppID               int64             `json:"app_id"`
	Account             Account           `json:"account"`
	RepositorySelection string            `json:"repository_selection"`
	Permissions         map[string]string `json:"permissions"`
	Events              []string          `json:"events"`
	SuspendedAt         *time.Time        `json:"suspended_at"`
}

type Repository struct {
	ID            int64   `json:"id"`
	NodeID        string  `json:"node_id"`
	Name          string  `json:"name"`
	FullName      string  `json:"full_name"`
	Owner         Account `json:"owner"`
	DefaultBranch string  `json:"default_branch"`
	Visibility    string  `json:"visibility"`
	HTMLURL       string  `json:"html_url"`
	Private       bool    `json:"private"`
}

type AppAuthentication struct {
	AppID         int64
	PrivateKeyPEM string
}

func (c *Client) ExchangeManifestCode(ctx context.Context, code string) (ManifestApp, error) {
	var app ManifestApp
	if strings.TrimSpace(code) == "" {
		return app, errors.New("GitHub manifest code is required")
	}
	path := "/app-manifests/" + url.PathEscape(code) + "/conversions"
	if err := c.request(ctx, http.MethodPost, path, "", nil, &app); err != nil {
		return app, fmt.Errorf("exchange GitHub manifest code: %w", err)
	}
	return app, nil
}

func (c *Client) LookupAccount(ctx context.Context, login string) (Account, error) {
	var account Account
	login = strings.TrimSpace(login)
	if login == "" || strings.ContainsAny(login, "/?#") {
		return account, errors.New("GitHub account login is required")
	}
	path := "/users/" + url.PathEscape(login)
	if err := c.request(ctx, http.MethodGet, path, "", nil, &account); err != nil {
		return account, fmt.Errorf("look up GitHub account: %w", err)
	}
	if account.ID <= 0 || account.Login == "" || account.Type == "" {
		return account, errors.New("GitHub returned an invalid account")
	}
	return account, nil
}

func (c *Client) GetInstallation(
	ctx context.Context,
	auth AppAuthentication,
	installationID int64,
) (Installation, error) {
	var installation Installation
	jwt, err := appJWT(auth)
	if err != nil {
		return installation, err
	}
	path := "/app/installations/" + strconv.FormatInt(installationID, 10)
	if err := c.request(ctx, http.MethodGet, path, "Bearer "+jwt, nil, &installation); err != nil {
		return installation, fmt.Errorf("load GitHub installation: %w", err)
	}
	return installation, nil
}

func (c *Client) ListInstallationRepositories(
	ctx context.Context,
	auth AppAuthentication,
	installationID int64,
) ([]Repository, error) {
	token, err := c.createInstallationToken(ctx, auth, installationID)
	if err != nil {
		return nil, err
	}

	repositories := make([]Repository, 0)
	for page := 1; ; page++ {
		var response struct {
			Repositories []Repository `json:"repositories"`
		}
		path := "/installation/repositories?per_page=100&page=" + strconv.Itoa(page)
		if err := c.request(
			ctx,
			http.MethodGet,
			path,
			"Bearer "+token,
			nil,
			&response,
		); err != nil {
			return nil, fmt.Errorf("list GitHub installation repositories page %d: %w", page, err)
		}
		repositories = append(repositories, response.Repositories...)
		if len(response.Repositories) < 100 {
			break
		}
	}
	return repositories, nil
}

func (c *Client) ResolveRevision(
	ctx context.Context,
	auth AppAuthentication,
	installationID int64,
	repository, reference string,
) (string, error) {
	repository = strings.Trim(strings.TrimSpace(repository), "/")
	reference = strings.TrimSpace(reference)
	if installationID <= 0 || strings.Count(repository, "/") != 1 || reference == "" {
		return "", errors.New("GitHub repository and reference are required")
	}
	token, err := c.createInstallationToken(ctx, auth, installationID)
	if err != nil {
		return "", err
	}
	var commit struct {
		SHA string `json:"sha"`
	}
	path := "/repos/" + repository + "/commits/" + url.PathEscape(reference)
	if err := c.request(ctx, http.MethodGet, path, "Bearer "+token, nil, &commit); err != nil {
		return "", fmt.Errorf("resolve GitHub revision: %w", err)
	}
	if len(commit.SHA) != 40 {
		return "", errors.New("GitHub returned an invalid commit revision")
	}
	return strings.ToLower(commit.SHA), nil
}

func (c *Client) GetFileContent(
	ctx context.Context,
	auth AppAuthentication,
	installationID int64,
	repository, filePath, reference string,
) ([]byte, bool, error) {
	repository = strings.Trim(strings.TrimSpace(repository), "/")
	filePath = strings.TrimPrefix(strings.TrimSpace(filePath), "/")
	reference = strings.TrimSpace(reference)
	if installationID <= 0 || strings.Count(repository, "/") != 1 || filePath == "" ||
		reference == "" {
		return nil, false, errors.New("GitHub repository, file path, and reference are required")
	}
	token, err := c.createInstallationToken(ctx, auth, installationID)
	if err != nil {
		return nil, false, err
	}
	segments := strings.Split(filePath, "/")
	for index, segment := range segments {
		segments[index] = url.PathEscape(segment)
	}
	requestPath := "/repos/" + repository + "/contents/" + strings.Join(segments, "/") +
		"?ref=" + url.QueryEscape(reference)
	var response struct {
		Type     string `json:"type"`
		Encoding string `json:"encoding"`
		Content  string `json:"content"`
	}
	if err := c.request(
		ctx,
		http.MethodGet,
		requestPath,
		"Bearer "+token,
		nil,
		&response,
	); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("load GitHub file %s: %w", filePath, err)
	}
	if response.Type != "file" || response.Encoding != "base64" {
		return nil, false, errors.New("GitHub returned an unsupported content entry")
	}
	payload, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(response.Content, "\n", ""))
	if err != nil {
		return nil, false, fmt.Errorf("decode GitHub file %s: %w", filePath, err)
	}
	if len(payload) > maxResponseBody {
		return nil, false, errors.New("GitHub file exceeded the allowed size")
	}
	return payload, true, nil
}

func (c *Client) createInstallationToken(
	ctx context.Context,
	auth AppAuthentication,
	installationID int64,
) (string, error) {
	jwt, err := appJWT(auth)
	if err != nil {
		return "", err
	}
	var response struct {
		Token string `json:"token"`
	}
	path := "/app/installations/" + strconv.FormatInt(installationID, 10) + "/access_tokens"
	if err := c.request(ctx, http.MethodPost, path, "Bearer "+jwt, nil, &response); err != nil {
		return "", fmt.Errorf("create GitHub installation token: %w", err)
	}
	if strings.TrimSpace(response.Token) == "" {
		return "", errors.New("GitHub returned an empty installation token")
	}
	return response.Token, nil
}

func (c *Client) request(
	ctx context.Context,
	method, path, authorization string,
	body io.Reader,
	destination any,
) error {
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", APIVersion)
	request.Header.Set("User-Agent", "DeployCrate-CE")
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, maxResponseBody+1)
	payload, err := io.ReadAll(limited)
	if err != nil {
		return err
	}
	if len(payload) > maxResponseBody {
		return errors.New("GitHub response exceeded the allowed size")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		switch response.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			if response.StatusCode == http.StatusForbidden &&
				response.Header.Get("X-RateLimit-Remaining") == "0" {
				return ErrRateLimited
			}
			return ErrUnauthorized
		case http.StatusNotFound:
			return ErrNotFound
		default:
			return fmt.Errorf("GitHub API returned status %d", response.StatusCode)
		}
	}
	if destination == nil || len(payload) == 0 {
		return nil
	}
	if err := json.Unmarshal(payload, destination); err != nil {
		return fmt.Errorf("decode GitHub response: %w", err)
	}
	return nil
}

func appJWT(auth AppAuthentication) (string, error) {
	if auth.AppID <= 0 {
		return "", errors.New("GitHub App ID is required")
	}
	privateKey, err := parseRSAPrivateKey(auth.PrivateKeyPEM)
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	claims, _ := json.Marshal(
		map[string]any{
			"iat": now.Add(-60 * time.Second).Unix(),
			"exp": now.Add(9 * time.Minute).Unix(),
			"iss": strconv.FormatInt(auth.AppID, 10),
		},
	)
	unsigned := base64.RawURLEncoding.EncodeToString(
		header,
	) + "." + base64.RawURLEncoding.EncodeToString(
		claims,
	)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign GitHub App JWT: %w", err)
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func ValidateAppAuthentication(auth AppAuthentication) error {
	_, err := appJWT(auth)
	return err
}

func parseRSAPrivateKey(value string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(value))
	if block == nil {
		return nil, errors.New("GitHub App private key is not valid PEM")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("GitHub App private key is invalid")
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("GitHub App private key must be RSA")
	}
	return key, nil
}
