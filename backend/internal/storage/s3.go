package storage

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type S3 struct {
	Client     *s3.Client
	Bucket     string
	CDNBaseURL string
	ServiceURL string
}

type Options struct {
	AccessKey      string
	SecretKey      string
	ServiceURL     string
	Region         string
	Bucket         string
	ForcePathStyle bool
	VerifySSL      bool
	CDNBaseURL     string
}

func New(ctx context.Context, o Options) (*S3, error) {
	if o.AccessKey == "" || o.SecretKey == "" || o.ServiceURL == "" || o.Bucket == "" {
		return nil, fmt.Errorf("s3 configuration incomplete")
	}

	httpClient := awshttp.NewBuildableClient().
		WithTimeout(5 * time.Minute).
		WithTransportOptions(func(tr *http.Transport) {
			tr.Proxy = http.ProxyFromEnvironment
			tr.MaxIdleConns = 100
			tr.MaxIdleConnsPerHost = 20
			tr.IdleConnTimeout = 90 * time.Second
			tr.TLSHandshakeTimeout = 15 * time.Second
			tr.ResponseHeaderTimeout = 2 * time.Minute
			tr.ForceAttemptHTTP2 = true
			if tr.TLSClientConfig == nil {
				tr.TLSClientConfig = &tls.Config{}
			}
			tr.TLSClientConfig.MinVersion = tls.VersionTLS12
			tr.TLSClientConfig.InsecureSkipVerify = !o.VerifySSL //nolint:gosec
		})

	region := o.Region
	if region == "" {
		region = "auto"
	}

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(o.AccessKey, o.SecretKey, "")),
		config.WithHTTPClient(httpClient),
	)
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(cfg, func(opts *s3.Options) {
		opts.BaseEndpoint = aws.String(strings.TrimRight(o.ServiceURL, "/"))
		opts.UsePathStyle = o.ForcePathStyle
	})

	return &S3{
		Client:     client,
		Bucket:     o.Bucket,
		CDNBaseURL: strings.TrimRight(o.CDNBaseURL, "/"),
		ServiceURL: strings.TrimRight(o.ServiceURL, "/"),
	}, nil
}

func (s *S3) Upload(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	_, err := s.Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.Bucket),
		Key:           aws.String(key),
		Body:          r,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(contentType),
		ACL:           types.ObjectCannedACLPublicRead,
	})
	return err
}

func (s *S3) Delete(ctx context.Context, key string) error {
	_, err := s.Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(key),
	})
	if err != nil && isNotFound(err) {
		return nil
	}
	return err
}

func (s *S3) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(key),
	})
	if err == nil {
		return true, nil
	}
	if isNotFound(err) {
		return false, nil
	}
	return false, err
}

func (s *S3) PublicURL(key string) string {
	if s.CDNBaseURL != "" {
		return s.CDNBaseURL + "/" + key
	}
	return s.ServiceURL + "/" + s.Bucket + "/" + key
}

func (s *S3) List(ctx context.Context, fn func(key string) bool) (bool, error) {
	paginator := s3.NewListObjectsV2Paginator(s.Client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.Bucket),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return false, err
		}
		for _, obj := range page.Contents {
			if obj.Key == nil {
				continue
			}
			if !fn(*obj.Key) {
				return false, nil
			}
		}
	}
	return true, nil
}

func isNotFound(err error) bool {
	var apiErr *awshttp.ResponseError
	if errors.As(err, &apiErr) && apiErr.HTTPStatusCode() == http.StatusNotFound {
		return true
	}
	var nf *types.NotFound
	if errors.As(err, &nf) {
		return true
	}
	var nsk *types.NoSuchKey
	return errors.As(err, &nsk)
}
