package logic

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"gitee.com/we7coreteam/w7-cdn-cache/common/helper"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/panjf2000/ants/v2"
)

// transferMap tracks in-flight uploads and prevents duplicate uploads for a
// host/bucket/key combination. Expiration deletes are intentionally independent
// because deleting a cache entry is safe; a later request can repopulate it.
var transferMap sync.Map
var transferChan chan TransferInfo
var transferPool *ants.PoolWithFunc

func init() {
	transferChan = make(chan TransferInfo, 1000)

	pool, err := ants.NewPoolWithFunc(100, func(transfer interface{}) {
		Transfer{}.transfer(transfer.(TransferInfo))
	})
	if err != nil {
		panic(err)
	}
	transferPool = pool
}

type TransferInfo struct {
	Host          string
	RemoteUrl     string
	S3Path        string
	CacheTTL      int64
	ResourceSize  int64
	ContentType   string
	CacheSetting  StorageCacheSetting
	SourceHeaders http.Header
}

type Transfer struct {
	logic
}

func (l Transfer) Push(transferInfo TransferInfo) {
	key := transferKey(transferInfo)
	if _, exists := transferMap.LoadOrStore(key, struct{}{}); exists {
		return
	}
	transferChan <- transferInfo
}

func (l Transfer) transfer(transferInfo TransferInfo) {
	slog.Info("transfer begin", "host", transferInfo.Host, "remote_url", transferInfo.RemoteUrl, "s3_path", transferInfo.S3Path)

	defer transferMap.Delete(transferKey(transferInfo))

	ctx := context.Background()
	s3Bucket := aws.String(transferInfo.CacheSetting.StorageCacheS3.Bucket)
	s3Key := aws.String(transferInfo.S3Path)

	s3Client := S3Client{}.GetClient(transferInfo.Host)
	if s3Client == nil {
		slog.Error("GetClient", "err", "GetClient failed", "host", transferInfo.Host)
		return
	}
	createInput := &s3.CreateMultipartUploadInput{
		Bucket: s3Bucket,
		Key:    s3Key,
	}
	if transferInfo.ContentType != "" {
		createInput.ContentType = aws.String(transferInfo.ContentType)
	}
	createOutput, err := s3Client.CreateMultipartUpload(ctx, createInput)
	if err != nil {
		slog.Error("CreateMultipartUpload", "host", transferInfo.Host, "s3_path", transferInfo.S3Path, "err", err)
		return
	}
	if createOutput == nil {
		slog.Error("CreateMultipartUpload", "host", transferInfo.Host, "s3_path", transferInfo.S3Path, "err", "empty response")
		return
	}
	uploadID := aws.ToString(createOutput.UploadId)
	if uploadID == "" {
		slog.Error("CreateMultipartUpload", "host", transferInfo.Host, "s3_path", transferInfo.S3Path, "err", "empty upload id")
		return
	}

	var chunkUploadErr error
	var completeParts []types.CompletedPart
	chunks := helper.CalculateChunks(transferInfo.ResourceSize, 5<<20)
	for partNumber, item := range chunks {
		chunkBuffer := bytes.Buffer{}

		chunkUploadErr = Storage{}.DownloadChunk(ctx, Storage{}.DownloadChunkByHttp(), transferInfo.RemoteUrl, item.Start, item.End, transferInfo.SourceHeaders, &chunkBuffer)
		if chunkUploadErr != nil {
			slog.Error("DownloadChunkByHttp", "host", transferInfo.Host, "remote_url", transferInfo.RemoteUrl, "s3_path", transferInfo.S3Path, "chunk", item, "err", chunkUploadErr)
			break
		}

		part, uploadErr := s3Client.UploadPart(ctx, &s3.UploadPartInput{
			Bucket:        s3Bucket,
			Key:           s3Key,
			UploadId:      aws.String(uploadID),
			PartNumber:    aws.Int32(int32(partNumber + 1)),
			Body:          bytes.NewReader(chunkBuffer.Bytes()),
			ContentLength: aws.Int64(int64(chunkBuffer.Len())),
		})
		chunkUploadErr = uploadErr
		if chunkUploadErr != nil {
			slog.Error("UploadPart", "host", transferInfo.Host, "s3_path", transferInfo.S3Path, "err", chunkUploadErr)
			break
		}
		completeParts = append(completeParts, types.CompletedPart{
			PartNumber: aws.Int32(int32(partNumber + 1)),
			ETag:       part.ETag,
		})
	}

	if chunkUploadErr != nil {
		_, _ = s3Client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
			Bucket:   s3Bucket,
			Key:      s3Key,
			UploadId: aws.String(uploadID),
		})
		return
	}

	completeOutput, err := s3Client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   s3Bucket,
		Key:      s3Key,
		UploadId: aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: completeParts,
		},
	})
	if err != nil {
		slog.Error("CompleteMultipartUpload", "host", transferInfo.Host, "s3_path", transferInfo.S3Path, "err", err)
		return
	}
	if completeOutput == nil {
		slog.Error("CompleteMultipartUpload", "host", transferInfo.Host, "s3_path", transferInfo.S3Path, "err", "empty response")
		return
	}

	if ttl, ok := cacheTTLDuration(transferInfo.CacheTTL); ok {
		expireAt := time.Now().Add(ttl)
		if err := EnqueueCacheExpiration(CacheExpirationTask{
			Host:               transferInfo.Host,
			Bucket:             transferInfo.CacheSetting.StorageCacheS3.Bucket,
			Key:                transferInfo.S3Path,
			VersionID:          aws.ToString(completeOutput.VersionId),
			StorageFingerprint: storageConfigFingerprint(*transferInfo.CacheSetting.StorageCacheS3),
			ExpireAt:           expireAt,
		}); err != nil {
			slog.Error("enqueue cache expiration", "host", transferInfo.Host, "s3_path", transferInfo.S3Path, "err", err)
		}
	}

	slog.Info("transfer CompleteMultipartUpload", "host", transferInfo.Host, "s3_path", transferInfo.S3Path)
}

func transferKey(transferInfo TransferInfo) string {
	bucket := ""
	if transferInfo.CacheSetting.StorageCacheS3 != nil {
		bucket = transferInfo.CacheSetting.StorageCacheS3.Bucket
	}
	return transferInfo.Host + "\x00" + bucket + "\x00" + transferInfo.S3Path
}

func (l Transfer) Loop() {
	go func() {
		for {
			select {
			case transferInfo := <-transferChan:
				err := transferPool.Invoke(transferInfo)
				if err != nil {
					slog.Error("transferPool Invoke", "host", transferInfo.Host, "s3_path", transferInfo.S3Path, "err", err)
					return
				}
			}
		}
	}()
}
