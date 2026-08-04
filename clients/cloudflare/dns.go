package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
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
	cloudflareAPIBaseURL = "https://api.cloudflare.com/client/v4"
	cloudflareBodyLimit  = 2 << 20
)

var (
	ErrUnauthorized = errors.New("Cloudflare rejected the API token")
	ErrNotFound     = errors.New("Cloudflare resource not found")
)

type DNS struct {
	httpClient *http.Client
	baseURL    string
}

type Zone struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type DNSRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
	Comment string `json:"comment"`
}

type DNSRecordInput struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
	Comment string `json:"comment,omitempty"`
}

type responseEnvelope[T any] struct {
	Success bool `json:"success"`
	Result  T    `json:"result"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	ResultInfo struct {
		Page       int `json:"page"`
		TotalPages int `json:"total_pages"`
	} `json:"result_info"`
}

func NewDNS() *DNS {
	return &DNS{httpClient: telemetry.NewHTTPClient(20 * time.Second), baseURL: cloudflareAPIBaseURL}
}

func (client *DNS) VerifyAccountToken(ctx context.Context, accountID, token string) error {
	var result struct {
		Status string `json:"status"`
	}
	path := "/accounts/" + url.PathEscape(accountID) + "/tokens/verify"
	if err := client.request(ctx, http.MethodGet, path, token, nil, &result); err != nil {
		return fmt.Errorf("verify Cloudflare account-owned API token: %w", err)
	}
	if result.Status != "active" {
		return ErrUnauthorized
	}
	return nil
}

func (client *DNS) ListZones(ctx context.Context, accountID, token string) ([]Zone, error) {
	zones := make([]Zone, 0)
	for page := 1; ; page++ {
		var envelope responseEnvelope[[]Zone]
		query := url.Values{"account.id": {accountID}, "per_page": {"50"}, "page": {strconv.Itoa(page)}}
		path := "/zones?" + query.Encode()
		if err := client.requestEnvelope(ctx, http.MethodGet, path, token, nil, &envelope); err != nil {
			return nil, fmt.Errorf("list Cloudflare zones: %w", err)
		}
		zones = append(zones, envelope.Result...)
		if envelope.ResultInfo.TotalPages <= page || len(envelope.Result) == 0 {
			break
		}
	}
	return zones, nil
}

func (client *DNS) ListAddressRecords(ctx context.Context, token, zoneID, hostname string) ([]DNSRecord, error) {
	records := make([]DNSRecord, 0)
	for page := 1; ; page++ {
		query := url.Values{"name": {hostname}, "per_page": {"100"}, "page": {strconv.Itoa(page)}}
		var envelope responseEnvelope[[]DNSRecord]
		path := "/zones/" + url.PathEscape(zoneID) + "/dns_records?" + query.Encode()
		if err := client.requestEnvelope(ctx, http.MethodGet, path, token, nil, &envelope); err != nil {
			return nil, fmt.Errorf("list Cloudflare address records: %w", err)
		}
		for _, record := range envelope.Result {
			if strings.EqualFold(record.Type, "A") || strings.EqualFold(record.Type, "CNAME") {
				records = append(records, record)
			}
		}
		if envelope.ResultInfo.TotalPages <= page || len(envelope.Result) == 0 {
			break
		}
	}
	return records, nil
}

func (client *DNS) CreateARecord(ctx context.Context, token, zoneID string, input DNSRecordInput) (DNSRecord, error) {
	var record DNSRecord
	path := "/zones/" + url.PathEscape(zoneID) + "/dns_records"
	if err := client.request(ctx, http.MethodPost, path, token, input, &record); err != nil {
		return DNSRecord{}, fmt.Errorf("create Cloudflare A record: %w", err)
	}
	return record, nil
}

func (client *DNS) UpdateARecord(ctx context.Context, token, zoneID, recordID string, input DNSRecordInput) (DNSRecord, error) {
	var record DNSRecord
	path := "/zones/" + url.PathEscape(zoneID) + "/dns_records/" + url.PathEscape(recordID)
	if err := client.request(ctx, http.MethodPatch, path, token, input, &record); err != nil {
		return DNSRecord{}, fmt.Errorf("update Cloudflare A record: %w", err)
	}
	return record, nil
}

func (client *DNS) DeleteRecord(ctx context.Context, token, zoneID, recordID string) error {
	path := "/zones/" + url.PathEscape(zoneID) + "/dns_records/" + url.PathEscape(recordID)
	if err := client.request(ctx, http.MethodDelete, path, token, nil, nil); err != nil && !errors.Is(err, ErrNotFound) {
		return fmt.Errorf("delete Cloudflare DNS record: %w", err)
	}
	return nil
}

func (client *DNS) request(ctx context.Context, method, path, token string, body, result any) error {
	envelope := responseEnvelope[json.RawMessage]{}
	if err := client.requestEnvelope(ctx, method, path, token, body, &envelope); err != nil {
		return err
	}
	if result == nil || len(envelope.Result) == 0 || string(envelope.Result) == "null" {
		return nil
	}
	if err := json.Unmarshal(envelope.Result, result); err != nil {
		return fmt.Errorf("decode Cloudflare result: %w", err)
	}
	return nil
}

func (client *DNS) requestEnvelope(ctx context.Context, method, path, token string, body any, destination any) error {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, client.baseURL+path, payload)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	request.Header.Set("User-Agent", "DeployCrate-CE")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, cloudflareBodyLimit+1))
	if err != nil {
		return err
	}
	if len(data) > cloudflareBodyLimit {
		return errors.New("Cloudflare response exceeded the allowed size")
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return ErrUnauthorized
	}
	if response.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Cloudflare API returned status %d", response.StatusCode)
	}
	var status struct {
		Success bool `json:"success"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(data, &status); err != nil {
		return fmt.Errorf("decode Cloudflare response status: %w", err)
	}
	if !status.Success {
		message := "Cloudflare API rejected the request"
		if len(status.Errors) > 0 && strings.TrimSpace(status.Errors[0].Message) != "" {
			message = strings.TrimSpace(status.Errors[0].Message)
		}
		return errors.New(message)
	}
	if err := json.Unmarshal(data, destination); err != nil {
		return fmt.Errorf("decode Cloudflare response: %w", err)
	}
	return nil
}
