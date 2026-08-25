package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Client struct {
	mc        *minio.Client
	bucket    string
	publicURL string
}

func New(endpoint, accessKey, secretKey, bucket, publicURL string, useSSL bool) (*Client, error) {
	mc, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	exists, err := mc.BucketExists(ctx, bucket)
	if err != nil {
		return nil, err
	}
	if !exists {
		if err := mc.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, err
		}
	}

	policy := fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":["*"]},"Action":["s3:GetObject"],"Resource":["arn:aws:s3:::%s/*"]}]}`, bucket)
	_ = mc.SetBucketPolicy(ctx, bucket, policy)

	if publicURL == "" {
		scheme := "http"
		if useSSL {
			scheme = "https"
		}
		publicURL = fmt.Sprintf("%s://%s", scheme, endpoint)
	}

	return &Client{mc: mc, bucket: bucket, publicURL: publicURL}, nil
}

func (c *Client) Upload(ctx context.Context, objectName, contentType string, r io.Reader, size int64) (string, error) {
	_, err := c.mc.PutObject(ctx, c.bucket, objectName, r, size, minio.PutObjectOptions{
		ContentType:        contentType,
		ContentDisposition: "inline",
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s/%s", c.publicURL, c.bucket, objectName), nil
}

func (c *Client) ReadAll(ctx context.Context, objectName string, maxBytes int64) ([]byte, error) {
	object, err := c.mc.GetObject(ctx, c.bucket, objectName, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer object.Close()
	data, err := io.ReadAll(io.LimitReader(object, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("object exceeds size limit")
	}
	return data, nil
}

func (c *Client) ObjectKeyFromURL(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	base, _ := url.Parse(strings.TrimRight(c.publicURL, "/"))
	if base == nil || base.Host == "" || !strings.EqualFold(u.Host, base.Host) {
		return ""
	}
	path := strings.TrimPrefix(u.Path, "/")
	basePath := strings.Trim(strings.TrimPrefix(base.Path, "/"), "/")
	if basePath != "" {
		path = strings.TrimPrefix(path, basePath+"/")
	}
	return strings.TrimPrefix(path, c.bucket+"/")
}
