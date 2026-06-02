package logic

import (
	"net"
	"net/http"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var defaultMinioClientMap = make(map[string]*minio.Core)

type S3Client struct {
	logic
}

func (l S3Client) ResetMinioClient(host string, config StorageMinio) error {
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
		ExpectContinueTimeout: 1 * time.Second,
		MaxConnsPerHost:       10000, // 限制每个主机的总连接数
	}

	// 初始化MinIO客户端时传入自定义Transport
	client, err := minio.NewCore(config.Endpoint, &minio.Options{
		Creds:     credentials.NewStaticV4(config.AccessKey, config.SecretKey, ""),
		Transport: customTransport,
	})
	if err != nil {
		return err
	}

	defaultMinioClientMap[host] = client
	return nil
}

func (l S3Client) GetMinioClient(host string) *minio.Core {
	client, ok := defaultMinioClientMap[host]
	if !ok {
		return nil
	}
	return client
}
