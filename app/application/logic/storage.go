package logic

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gitee.com/we7coreteam/w7-cdn-cache/common/helper"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const (
	DownloadChunkSize     = 2 << 20 // 分片大小 2MB
	DownloadMaxRetry      = 3       // 单个分片最大重试次数
	DownloadRetryInterval = 50 * time.Millisecond
)

type ChunkDownloadFunc func(ctx context.Context, path string, start, end int64, headers http.Header) (io.ReadCloser, error)

type ObjectInfo struct {
	Name         string      `json:"name,omitempty"`
	Key          string      `json:"key" json:"key,omitempty"`                  // Name of the object
	LastModified time.Time   `json:"lastModified" json:"last_modified"`         // Date and time the object was last modified.
	Size         int64       `json:"size" json:"size,omitempty"`                // Size in bytes of the object.
	ContentType  string      `json:"contentType" json:"content_type,omitempty"` // A standard MIME type describing the format of the object data.
	Header       http.Header `json:"header,omitempty"`
}

type Storage struct {
	logic
}

func (l Storage) GetObjectInfoByS3(ctx context.Context, client *s3.Client, bucket string, objectPath string) (*ObjectInfo, error) {
	object, err := client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(objectPath),
	})
	if err != nil {
		return nil, err
	}

	contentLength := int64(0)
	if object.ContentLength != nil {
		contentLength = *object.ContentLength
	}
	contentType := ""
	if object.ContentType != nil {
		contentType = *object.ContentType
	}
	lastModified := time.Time{}
	if object.LastModified != nil {
		lastModified = *object.LastModified
	}
	headers := make(http.Header)
	headers.Set("Content-Length", fmt.Sprintf("%d", contentLength))
	if contentType != "" {
		headers.Set("Content-Type", contentType)
	}
	if object.ETag != nil {
		headers.Set("ETag", *object.ETag)
	}
	if !lastModified.IsZero() {
		headers.Set("Last-Modified", lastModified.Format(time.RFC1123))
	}
	if object.Expires != nil {
		headers.Set("Expires", object.Expires.Format(time.RFC1123))
	}

	return &ObjectInfo{
		Name:         filepath.Base(objectPath),
		Key:          objectPath,
		LastModified: lastModified,
		ContentType:  contentType,
		Size:         contentLength,
		Header:       headers,
	}, nil
}

func (l Storage) DownloadChunkByS3(client *s3.Client, bucket string) ChunkDownloadFunc {
	return func(ctx context.Context, path string, start, end int64, headers http.Header) (io.ReadCloser, error) {
		obj, err := client.GetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(path),
			Range:  aws.String(fmt.Sprintf("bytes=%d-%d", start, end)),
		})
		if err != nil {
			return nil, fmt.Errorf("get object failed: %w", err)
		}
		if obj == nil || obj.Body == nil {
			return nil, errors.New("get object returned an empty body")
		}
		return obj.Body, nil
	}
}

func (l Storage) GetObjectInfoByHttp(ctx context.Context, remoteUrl string, headers http.Header) (*ObjectInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// HEAD请求获取文件信息
	req, err := http.NewRequestWithContext(ctx, "HEAD", remoteUrl, nil)
	if err != nil {
		return nil, err
	}

	if headers != nil {
		for key, val := range headers {
			req.Header.Set(key, val[0])
		}
		if host := headers.Get("Host"); host != "" {
			req.Host = host
			req.Header.Del("Host")
		}
	}
	// Metadata requests must not inherit the client's Range. A chained proxy
	// may otherwise answer HEAD with 206 and report only the requested part.
	req.Header.Del("Range")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HEAD请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("服务器返回状态码: %d", resp.StatusCode)
	}

	objectInfo := &ObjectInfo{}
	urlInfo, _ := url.Parse(remoteUrl)
	if urlInfo != nil {
		objectInfo.Name = strings.TrimLeft(path.Base(urlInfo.Path), "/")
		objectInfo.Key = objectInfo.Name
	}

	// 检查文件大小
	contentLength := resp.Header.Get("Content-Length")
	if contentLength != "" {
		fileSize, err := strconv.ParseInt(contentLength, 10, 64)
		if err == nil {
			objectInfo.Size = fileSize
		}
	}
	objectInfo.ContentType = resp.Header.Get("Content-Type")
	objectInfo.Header = resp.Header

	return objectInfo, nil
}

func (l Storage) DownloadChunkByHttp() ChunkDownloadFunc {
	return func(ctx context.Context, url string, start, end int64, headers http.Header) (io.ReadCloser, error) {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}

		if headers != nil {
			for key, val := range headers {
				req.Header.Set(key, val[0])
			}
			if host := headers.Get("Host"); host != "" {
				req.Host = host
				req.Header.Del("Host")
			}
		}

		rangeHeader := fmt.Sprintf("bytes=%d-%d", start, end)
		req.Header.Set("Range", rangeHeader)

		// 发送请求
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}

		// 检查响应状态
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("无效响应状态: %d", resp.StatusCode)
		}

		return resp.Body, nil
	}
}

func (l Storage) DownloadChunk(ctx context.Context, chunkDownload ChunkDownloadFunc, path string, start, end int64, headers http.Header, targetWriter io.Writer) error {
	for chunkStart := start; chunkStart <= end; {
		chunkEnd := chunkStart + DownloadChunkSize
		if chunkEnd > end {
			chunkEnd = end
		}

		// 带有重试机制的分片下载
		success := false
		for attempt := 0; attempt < DownloadMaxRetry; attempt++ {
			if helper.CtxDone(ctx) {
				slog.Info("Download interrupted", "path", path)
				return errors.New("download interrupted")
			}

			writer, err := chunkDownload(ctx, path, chunkStart, chunkEnd, headers)
			if err == nil {
				_, copyErr := io.Copy(targetWriter, writer)
				closeErr := writer.Close()
				if copyErr != nil {
					return copyErr
				}
				if closeErr != nil {
					return closeErr
				}
				success = true
				break
			} else if attempt < DownloadMaxRetry-1 {
				slog.Info("Download Retrying chunk", "path", path, "start", chunkStart, "end", chunkEnd, "attempt", attempt)
				time.Sleep(DownloadRetryInterval)
			}
		}

		if !success {
			slog.Info("Download Chunk Failed", "path", path, "start", chunkStart, "end", chunkEnd)
			return errors.New("download chunk failed")
		}

		chunkStart = chunkEnd + 1
	}

	return nil
}
