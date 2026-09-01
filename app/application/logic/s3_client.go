package logic

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

var defaultS3ClientMap sync.Map

const s3ResponseHeaderTimeout = 30 * time.Second

type s3ClientEntry struct {
	client      *s3.Client
	fingerprint string
}

type S3Client struct {
	logic
}

func (l S3Client) ResetClient(host string, config StorageS3) error {
	customTransport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          1000, // 整个连接池的最大空闲连接数
		MaxIdleConnsPerHost:   50,   // 每个目标主机的最大空闲连接数
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: s3ResponseHeaderTimeout,
		ExpectContinueTimeout: 1 * time.Second,
		MaxConnsPerHost:       10000, // 限制每个主机的总连接数
	}

	awsConfig := aws.Config{
		Region:                     config.Region,
		Credentials:                credentials.NewStaticCredentialsProvider(config.AccessKey, config.SecretKey, ""),
		HTTPClient:                 &http.Client{Transport: customTransport},
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	}
	client := s3.NewFromConfig(awsConfig, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(config.Endpoint)
		// Most self-hosted S3-compatible services expect path-style addressing
		// instead of virtual-hosted bucket names.
		options.UsePathStyle = true
	})

	defaultS3ClientMap.Store(host, s3ClientEntry{
		client:      client,
		fingerprint: storageConfigFingerprint(config),
	})
	return nil
}

func (l S3Client) GetClient(host string) *s3.Client {
	client, _ := l.GetS3ClientWithFingerprint(host)
	return client
}

// GetS3ClientWithFingerprint returns the S3 client and the storage identity
// captured when that client was initialized.
func (l S3Client) GetS3ClientWithFingerprint(host string) (*s3.Client, string) {
	client, ok := defaultS3ClientMap.Load(host)
	if !ok {
		return nil, ""
	}
	entry, ok := client.(s3ClientEntry)
	if !ok {
		return nil, ""
	}
	return entry.client, entry.fingerprint
}

// storageConfigFingerprint identifies an S3 target without persisting its
// credentials in expiration tasks.
func storageConfigFingerprint(config StorageS3) string {
	hash := sha256.New()
	for _, value := range []string{
		config.Endpoint,
		config.AccessKey,
		config.SecretKey,
		config.Bucket,
		config.Region,
	} {
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(value))
	}
	return hex.EncodeToString(hash.Sum(nil))
}
