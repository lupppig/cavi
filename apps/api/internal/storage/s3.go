package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// S3Config configures an S3-compatible store.
type S3Config struct {
	// Endpoint is how the API reaches storage, e.g. http://localhost:8333 or
	// https://<account>.r2.cloudflarestorage.com.
	Endpoint string
	// PublicEndpoint is how browsers reach storage, used for signed URLs.
	// Defaults to Endpoint.
	PublicEndpoint  string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	// PathStyle addresses buckets as {endpoint}/{bucket} rather than
	// {bucket}.{endpoint}. Needed for local SeaweedFS.
	PathStyle bool
}

// S3 is a Store backed by an S3-compatible service.
type S3 struct {
	bucket  string
	client  *s3.Client
	presign *s3.PresignClient
}

func newClient(cfg S3Config, endpoint string) *s3.Client {
	return s3.New(s3.Options{
		Region:       cfg.Region,
		BaseEndpoint: aws.String(endpoint),
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		UsePathStyle: cfg.PathStyle,
		// Only compute checksums when an operation requires them; R2 and
		// SeaweedFS don't support every flexible-checksum algorithm, and a
		// presigned PUT must not demand a checksum header the browser
		// won't send.
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	})
}

// NewS3 returns an S3-backed Store.
func NewS3(cfg S3Config) *S3 {
	public := cfg.PublicEndpoint
	if public == "" {
		public = cfg.Endpoint
	}
	return &S3{
		bucket:  cfg.Bucket,
		client:  newClient(cfg, cfg.Endpoint),
		presign: s3.NewPresignClient(newClient(cfg, public)),
	}
}

// PresignUpload implements Store.
func (s *S3) PresignUpload(ctx context.Context, key, contentType, filename string, size int64, ttl time.Duration) (Upload, error) {
	req, err := s.presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:             aws.String(s.bucket),
		Key:                aws.String(key),
		ContentType:        aws.String(contentType),
		ContentDisposition: aws.String(attachment(filename)),
		ContentLength:      aws.Int64(size),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return Upload{}, fmt.Errorf("presign upload: %w", err)
	}
	// Browsers set Host and Content-Length themselves; pass on the rest.
	headers := map[string]string{}
	for name, values := range req.SignedHeader {
		switch http.CanonicalHeaderKey(name) {
		case "Host", "Content-Length":
			continue
		}
		headers[http.CanonicalHeaderKey(name)] = strings.Join(values, ",")
	}
	return Upload{URL: req.URL, Method: req.Method, Headers: headers, ExpiresAt: time.Now().Add(ttl)}, nil
}

// attachment builds a Content-Disposition header that downloads as
// filename. Non-ASCII names use RFC 2231 encoding.
func attachment(filename string) string {
	if f := mime.FormatMediaType("attachment", map[string]string{"filename": filename}); f != "" {
		return f
	}
	return "attachment"
}

// PresignDownload implements Store.
func (s *S3) PresignDownload(ctx context.Context, key string, ttl time.Duration) (string, time.Time, error) {
	req, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("presign download: %w", err)
	}
	return req.URL, time.Now().Add(ttl), nil
}

func isNotFound(err error) bool {
	var nsk *types.NoSuchKey
	var nf *types.NotFound
	if errors.As(err, &nsk) || errors.As(err, &nf) {
		return true
	}
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && (apiErr.ErrorCode() == "NotFound" || apiErr.ErrorCode() == "NoSuchKey")
}

// Stat implements Store.
func (s *S3) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		if isNotFound(err) {
			return ObjectInfo{}, ErrNotFound
		}
		return ObjectInfo{}, fmt.Errorf("stat object: %w", err)
	}
	return ObjectInfo{Size: aws.ToInt64(out.ContentLength)}, nil
}

// ReadHead implements Store.
func (s *S3) ReadHead(ctx context.Context, key string, n int64) ([]byte, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Range:  aws.String(fmt.Sprintf("bytes=0-%d", n-1)),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("read object: %w", err)
	}
	defer out.Body.Close()
	b, err := io.ReadAll(io.LimitReader(out.Body, n))
	if err != nil {
		return nil, fmt.Errorf("read object: %w", err)
	}
	return b, nil
}

// Delete implements Store.
func (s *S3) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil && !isNotFound(err) {
		return fmt.Errorf("delete object: %w", err)
	}
	return nil
}

// Setup creates the bucket if needed and allows browser uploads from
// origins. Intended for local development and first-time setup; it needs
// credentials with bucket administration rights.
func (s *S3) Setup(ctx context.Context, origins []string) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.bucket)})
	if err != nil {
		if _, err := s.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(s.bucket)}); err != nil {
			return fmt.Errorf("create bucket: %w", err)
		}
	}
	_, err = s.client.PutBucketCors(ctx, &s3.PutBucketCorsInput{
		Bucket: aws.String(s.bucket),
		CORSConfiguration: &types.CORSConfiguration{CORSRules: []types.CORSRule{{
			AllowedOrigins: origins,
			AllowedMethods: []string{http.MethodPut, http.MethodGet, http.MethodHead},
			AllowedHeaders: []string{"content-type", "content-disposition"},
			ExposeHeaders:  []string{"etag"},
			MaxAgeSeconds:  aws.Int32(3600),
		}}},
	})
	if err != nil {
		return fmt.Errorf("set bucket cors: %w", err)
	}
	return nil
}
