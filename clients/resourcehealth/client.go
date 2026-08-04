package resourcehealth

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const maximumHandshakeBytes = 1 << 20

type Endpoint struct {
	Address  string
	Port     int32
	Protocol string
}

type Credentials struct {
	Username string
	Password string
	Token    string
}

type HTTPOptions struct {
	Path           string
	ExpectedStatus int
	Credentials    Credentials
}

type Client struct{}

func New() Client { return Client{} }

func (Client) TCP(ctx context.Context, endpoint Endpoint) (string, error) {
	connection, err := dial(ctx, endpoint)
	if err != nil {
		return "", errors.New("the TCP connection failed")
	}
	_ = connection.Close()
	return "TCP connection accepted", nil
}

func (Client) HTTP(ctx context.Context, endpoint Endpoint, options HTTPOptions) (string, error) {
	protocol := strings.ToLower(strings.TrimSpace(endpoint.Protocol))
	if protocol == "clickhouse" {
		protocol = "http"
	}
	if protocol != "http" && protocol != "https" {
		return "", errors.New("the HTTP health check requires an HTTP or HTTPS endpoint")
	}
	requestPath := strings.TrimSpace(options.Path)
	if requestPath == "" {
		requestPath = "/"
	}
	reference, err := url.ParseRequestURI(requestPath)
	if err != nil || !strings.HasPrefix(reference.Path, "/") || reference.IsAbs() || reference.Host != "" {
		return "", errors.New("the HTTP health check path is invalid")
	}
	address, err := endpointAddress(endpoint)
	if err != nil {
		return "", err
	}
	target := (&url.URL{Scheme: protocol, Host: address}).ResolveReference(reference)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return "", errors.New("the HTTP health check request is invalid")
	}
	if strings.TrimSpace(options.Credentials.Username) != "" {
		request.SetBasicAuth(strings.TrimSpace(options.Credentials.Username), options.Credentials.Password)
	} else if options.Credentials.Token != "" {
		request.Header.Set("Authorization", "Bearer "+options.Credentials.Token)
	}
	transport := &http.Transport{}
	defer transport.CloseIdleConnections()
	response, err := (&http.Client{Transport: transport}).Do(request)
	if err != nil {
		return "", errors.New("the HTTP request failed")
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if options.ExpectedStatus > 0 {
		if response.StatusCode != options.ExpectedStatus {
			return "", fmt.Errorf("HTTP returned status %d instead of %d", response.StatusCode, options.ExpectedStatus)
		}
	} else if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("HTTP returned unhealthy status %d", response.StatusCode)
	}
	return fmt.Sprintf("HTTP responded %d", response.StatusCode), nil
}

func (Client) MySQL(ctx context.Context, endpoint Endpoint) (string, error) {
	connection, err := dial(ctx, endpoint)
	if err != nil {
		return "", errors.New("the MySQL connection failed")
	}
	defer connection.Close()
	header := make([]byte, 4)
	if _, err := io.ReadFull(connection, header); err != nil {
		return "", errors.New("the MySQL server did not send a handshake")
	}
	payloadLength := int(header[0]) | int(header[1])<<8 | int(header[2])<<16
	if payloadLength < 1 || payloadLength > maximumHandshakeBytes {
		return "", errors.New("the MySQL server sent an invalid handshake")
	}
	payload := make([]byte, payloadLength)
	if _, err := io.ReadFull(connection, payload); err != nil {
		return "", errors.New("the MySQL handshake was incomplete")
	}
	if payload[0] != 9 && payload[0] != 10 {
		return "", errors.New("the MySQL server sent an unsupported handshake")
	}
	return "MySQL handshake accepted", nil
}

func (Client) Redis(ctx context.Context, endpoint Endpoint, credentials Credentials) (string, error) {
	connection, err := dial(ctx, endpoint)
	if err != nil {
		return "", errors.New("the Redis connection failed")
	}
	defer connection.Close()
	reader := bufio.NewReaderSize(connection, 4096)
	if credentials.Password != "" {
		auth := []string{"AUTH", credentials.Password}
		if strings.TrimSpace(credentials.Username) != "" {
			auth = []string{"AUTH", strings.TrimSpace(credentials.Username), credentials.Password}
		}
		if err := writeRedisCommand(connection, auth...); err != nil {
			return "", errors.New("the Redis authentication request failed")
		}
		response, err := readRedisLine(reader)
		if err != nil || response != "+OK" {
			return "", errors.New("the Redis authentication failed")
		}
	}
	if err := writeRedisCommand(connection, "PING"); err != nil {
		return "", errors.New("the Redis PING request failed")
	}
	response, err := readRedisLine(reader)
	if err != nil || response != "+PONG" {
		return "", errors.New("the Redis server did not accept PING")
	}
	return "Redis responded PONG", nil
}

func dial(ctx context.Context, endpoint Endpoint) (net.Conn, error) {
	address, err := endpointAddress(endpoint)
	if err != nil {
		return nil, err
	}
	connection, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	if deadline, ok := ctx.Deadline(); ok {
		if err := connection.SetDeadline(deadline); err != nil {
			_ = connection.Close()
			return nil, err
		}
	}
	return connection, nil
}

func endpointAddress(endpoint Endpoint) (string, error) {
	address := strings.TrimSpace(endpoint.Address)
	if address == "" || endpoint.Port < 1 || endpoint.Port > 65535 {
		return "", errors.New("the health check endpoint is unavailable")
	}
	return net.JoinHostPort(address, strconv.Itoa(int(endpoint.Port))), nil
}

func writeRedisCommand(writer io.Writer, values ...string) error {
	if _, err := fmt.Fprintf(writer, "*%d\r\n", len(values)); err != nil {
		return err
	}
	for _, value := range values {
		if _, err := fmt.Fprintf(writer, "$%d\r\n%s\r\n", len(value), value); err != nil {
			return err
		}
	}
	return nil
}

func readRedisLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	if len(line) > 4096 {
		return "", errors.New("the Redis response is too large")
	}
	return strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"), nil
}
