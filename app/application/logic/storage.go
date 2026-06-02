package logic

import (
	"context"
	"errors"
	"fmt"
	"gitee.com/we7coreteam/w7-cdn-cache/common/helper"
	"github.com/minio/minio-go/v7"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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

func (l Storage) GetObjectInfoByMinio(ctx context.Context, client *minio.Core, bucket string, path string) (*ObjectInfo, error) {
	object, err := client.StatObject(ctx, bucket, path, minio.StatObjectOptions{})
	if err == nil {
		headers := make(http.Header)
		// 基础内容信息
		headers.Set("Content-Length", fmt.Sprintf("%d", object.Size))
		headers.Set("Content-Type", object.ContentType)
		// 缓存相关头
		headers.Set("ETag", object.ETag)
		headers.Set("Last-Modified", object.LastModified.Format(time.RFC1123))
		if !object.Expires.IsZero() {
			headers.Set("Expires", object.Expires.Format(time.RFC1123))
		}

		return &ObjectInfo{
			Name:         filepath.Base(object.Key),
			Key:          object.Key,
			LastModified: object.LastModified,
			ContentType:  object.ContentType,
			Size:         object.Size,
			Header:       headers,
		}, nil
	}

	return nil, err
}

func (l Storage) DownloadChunkByMinio(client *minio.Core, bucket string) ChunkDownloadFunc {
	return func(ctx context.Context, path string, start, end int64, headers http.Header) (io.ReadCloser, error) {
		opts := minio.GetObjectOptions{}
		err := opts.SetRange(start, end)
		if err != nil {
			return nil, fmt.Errorf("set range failed: %w", err)
		}

		obj, _, _, err := client.GetObject(ctx, bucket, path, opts)
		if err != nil {
			return nil, fmt.Errorf("get object failed: %w", err)
		}
		return obj, nil
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
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HEAD请求失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
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
				success = true
				_, err = io.Copy(targetWriter, writer)
				if err != nil {
					return err
				}
				if f, ok := writer.(http.Flusher); ok {
					f.Flush()
				}
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
