package objectstorage

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

const (
	ProviderS3 = "s3"
	ProviderR2 = "r2"
)

type Config struct {
	Provider       string
	Endpoint       string
	Region         string
	Bucket         string
	Prefix         string
	ForcePathStyle bool
}

type Credentials struct {
	AccessKeyID     string
	SecretAccessKey string
}

type ObjectInfo struct {
	Key      string
	Size     int64
	Metadata map[string]string
}

type Store interface {
	Put(context.Context, string, io.Reader, map[string]string) (ObjectInfo, error)
	Head(context.Context, string) (ObjectInfo, error)
	Get(context.Context, string, io.Writer) (ObjectInfo, error)
	List(context.Context, string) ([]ObjectInfo, error)
	Delete(context.Context, string) error
}

type Client struct {
	api    *s3.Client
	bucket string
	prefix string
}

func Normalize(config Config) (Config, error) {
	config.Provider = strings.ToLower(strings.TrimSpace(config.Provider))
	config.Endpoint = strings.TrimSpace(config.Endpoint)
	config.Region = strings.TrimSpace(config.Region)
	config.Bucket = strings.TrimSpace(config.Bucket)
	config.Prefix = strings.Trim(strings.TrimSpace(config.Prefix), "/")

	if config.Provider != ProviderS3 && config.Provider != ProviderR2 {
		return Config{}, fmt.Errorf("unsupported object storage provider %q", config.Provider)
	}
	if config.Bucket == "" {
		return Config{}, errors.New("object storage bucket is required")
	}
	if config.Provider == ProviderR2 {
		if config.Endpoint == "" {
			return Config{}, errors.New("Cloudflare R2 endpoint is required")
		}
		config.Region = "auto"
	}
	if config.Provider == ProviderS3 && config.Region == "" {
		return Config{}, errors.New("object storage region is required")
	}
	if config.Endpoint != "" {
		endpoint, err := url.Parse(config.Endpoint)
		if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" ||
			endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
			return Config{}, errors.New("object storage endpoint must be an absolute HTTPS URL")
		}
	}
	if config.Prefix != "" {
		for segment := range strings.SplitSeq(config.Prefix, "/") {
			if segment == "" || segment == "." || segment == ".." {
				return Config{}, errors.New("object storage prefix contains an unsafe path segment")
			}
		}
	}
	return config, nil
}

func New(ctx context.Context, config Config, credential Credentials) (*Client, error) {
	normalized, err := Normalize(config)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(credential.AccessKeyID) == "" ||
		strings.TrimSpace(credential.SecretAccessKey) == "" {
		return nil, errors.New("object storage credentials are required")
	}

	awsConfig, err := awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithRegion(normalized.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			credential.AccessKeyID,
			credential.SecretAccessKey,
			"",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("configure object storage client: %w", err)
	}
	api := s3.NewFromConfig(awsConfig, func(options *s3.Options) {
		options.UsePathStyle = normalized.ForcePathStyle
		if normalized.Endpoint != "" {
			options.BaseEndpoint = aws.String(normalized.Endpoint)
		}
	})
	return &Client{api: api, bucket: normalized.Bucket, prefix: normalized.Prefix}, nil
}

func (client *Client) Put(
	ctx context.Context,
	key string,
	body io.Reader,
	metadata map[string]string,
) (ObjectInfo, error) {
	key, err := client.key(key)
	if err != nil {
		return ObjectInfo{}, err
	}
	if _, err := client.api.PutObject(ctx, &s3.PutObjectInput{
		Bucket:   aws.String(client.bucket),
		Key:      aws.String(key),
		Body:     body,
		Metadata: metadata,
	}); err != nil {
		return ObjectInfo{}, fmt.Errorf("put object %q: %w", key, err)
	}
	return client.Head(ctx, strings.TrimPrefix(key, client.prefixWithSlash()))
}

func (client *Client) Head(ctx context.Context, key string) (ObjectInfo, error) {
	key, err := client.key(key)
	if err != nil {
		return ObjectInfo{}, err
	}
	output, err := client.api.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return ObjectInfo{}, fmt.Errorf("head object %q: %w", key, err)
	}
	return ObjectInfo{
		Key:      key,
		Size:     aws.ToInt64(output.ContentLength),
		Metadata: output.Metadata,
	}, nil
}

func (client *Client) Get(
	ctx context.Context,
	key string,
	destination io.Writer,
) (ObjectInfo, error) {
	key, err := client.key(key)
	if err != nil {
		return ObjectInfo{}, err
	}
	output, err := client.api.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return ObjectInfo{}, fmt.Errorf("get object %q: %w", key, err)
	}
	defer output.Body.Close()
	if _, err := io.Copy(destination, output.Body); err != nil {
		return ObjectInfo{}, fmt.Errorf("read object %q: %w", key, err)
	}
	return ObjectInfo{
		Key:      key,
		Size:     aws.ToInt64(output.ContentLength),
		Metadata: output.Metadata,
	}, nil
}

func (client *Client) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	prefix, err := client.key(prefix)
	if err != nil {
		return nil, err
	}
	paginator := s3.NewListObjectsV2Paginator(client.api, &s3.ListObjectsV2Input{
		Bucket: aws.String(client.bucket),
		Prefix: aws.String(prefix),
	})
	var objects []ObjectInfo
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list objects under %q: %w", prefix, err)
		}
		for _, object := range page.Contents {
			objects = append(
				objects,
				ObjectInfo{Key: aws.ToString(object.Key), Size: aws.ToInt64(object.Size)},
			)
		}
	}
	return objects, nil
}

func (client *Client) Delete(ctx context.Context, key string) error {
	key, err := client.key(key)
	if err != nil {
		return err
	}
	if _, err := client.api.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(key),
	}); err != nil {
		return fmt.Errorf("delete object %q: %w", key, err)
	}
	return nil
}

func (client *Client) Probe(ctx context.Context, installationID string) error {
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return fmt.Errorf("generate object storage probe nonce: %w", err)
	}
	nonce := hex.EncodeToString(nonceBytes)
	key := path.Join("probes", installationID, nonce)
	payload := []byte("deploycrate-ce-storage-probe:" + nonce)

	if _, err := client.Put(
		ctx,
		key,
		bytes.NewReader(payload),
		map[string]string{"probe": nonce},
	); err != nil {
		return fmt.Errorf("object storage probe write failed: %w", err)
	}
	defer client.Delete(context.WithoutCancel(ctx), key)
	if _, err := client.Head(ctx, key); err != nil {
		return fmt.Errorf("object storage probe head failed: %w", err)
	}
	var downloaded bytes.Buffer
	if _, err := client.Get(ctx, key, &downloaded); err != nil {
		return fmt.Errorf("object storage probe download failed: %w", err)
	}
	if !bytes.Equal(downloaded.Bytes(), payload) {
		return errors.New("object storage probe download did not match uploaded contents")
	}
	objects, err := client.List(ctx, path.Join("probes", installationID))
	if err != nil {
		return fmt.Errorf("object storage probe list failed: %w", err)
	}
	expectedKey, _ := client.key(key)
	found := false
	for _, object := range objects {
		found = found || object.Key == expectedKey
	}
	if !found {
		return errors.New("object storage probe object was absent from prefix listing")
	}
	if err := client.Delete(ctx, key); err != nil {
		return fmt.Errorf("object storage probe delete failed: %w", err)
	}
	for attempt := range 5 {
		_, err := client.Head(ctx, key)
		if IsNotFound(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("confirm object storage probe deletion: %w", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 200 * time.Millisecond):
		}
	}
	return errors.New("object storage probe remained after deletion")
}

func IsNotFound(err error) bool {
	var apiError smithy.APIError
	if !errors.As(err, &apiError) {
		return false
	}
	code := apiError.ErrorCode()
	return code == "NotFound" || code == "NoSuchKey" || code == "404"
}

func (client *Client) key(value string) (string, error) {
	value = strings.Trim(value, "/")
	for segment := range strings.SplitSeq(value, "/") {
		if segment == "." || segment == ".." {
			return "", errors.New("object key contains an unsafe path segment")
		}
	}
	if client.prefix == "" {
		return value, nil
	}
	if value == "" {
		return client.prefix, nil
	}
	return client.prefix + "/" + value, nil
}

func (client *Client) prefixWithSlash() string {
	if client.prefix == "" {
		return ""
	}
	return client.prefix + "/"
}
