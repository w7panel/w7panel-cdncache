package logic

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"sync"

	"gitee.com/we7coreteam/w7-cdn-cache/common/helper"
	"github.com/minio/minio-go/v7"
	"github.com/panjf2000/ants/v2"
)

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
	MinioPath     string
	ResourceSize  int64
	ContentType   string
	CacheSetting  StorageCacheSetting
	SourceHeaders http.Header
}

type Transfer struct {
	logic
}

func (l Transfer) Push(transferInfo TransferInfo) {
	_, exists := transferMap.LoadOrStore(transferInfo.MinioPath, transferInfo)
	if exists {
		return
	}

	transferChan <- transferInfo
}

func (l Transfer) transfer(transferInfo TransferInfo) {
	slog.Info("transfer begin", "transfer_info", transferInfo)

	defer transferMap.Delete(transferInfo.MinioPath)

	ctx := context.Background()

	minioClient := S3Client{}.GetMinioClient(transferInfo.Host)
	if minioClient == nil {
		slog.Error("GetMinioClient", "err", "GetMinioClient failed", "host", transferInfo.Host)
		return
	}
	uploadID, err := minioClient.NewMultipartUpload(ctx, transferInfo.CacheSetting.StorageCacheMinio.Bucket, transferInfo.MinioPath, minio.PutObjectOptions{
		ContentType: transferInfo.ContentType,
	})
	if err != nil {
		slog.Error("NewMultipartUpload", "transfer_info", transferInfo, "err", err)
		return
	}

	var chunkUploadErr error
	var completeParts []minio.CompletePart
	chunks := helper.CalculateChunks(transferInfo.ResourceSize, 5<<20)
	for partNumber, item := range chunks {
		chunkBuffer := bytes.Buffer{}

		chunkUploadErr = Storage{}.DownloadChunk(ctx, Storage{}.DownloadChunkByHttp(), transferInfo.RemoteUrl, item.Start, item.End, transferInfo.SourceHeaders, &chunkBuffer)
		if chunkUploadErr != nil {
			slog.Error("DownloadChunkByHttp", "transfer_info", transferInfo, "chunk", item, "err", chunkUploadErr)
			break
		}

		var part minio.ObjectPart
		part, chunkUploadErr = minioClient.PutObjectPart(ctx, transferInfo.CacheSetting.StorageCacheMinio.Bucket, transferInfo.MinioPath, uploadID, partNumber+1,
			&chunkBuffer, int64(chunkBuffer.Len()), minio.PutObjectPartOptions{})
		if chunkUploadErr != nil {
			slog.Error("PutObjectPart", "transfer_info", transferInfo, "err", chunkUploadErr)
			break
		}
		completeParts = append(completeParts, minio.CompletePart{
			PartNumber: part.PartNumber,
			ETag:       part.ETag,
		})
	}

	if chunkUploadErr != nil {
		_ = minioClient.AbortMultipartUpload(ctx, transferInfo.CacheSetting.StorageCacheMinio.Bucket, transferInfo.MinioPath, uploadID)
		return
	}

	_, err = minioClient.CompleteMultipartUpload(ctx, transferInfo.CacheSetting.StorageCacheMinio.Bucket, transferInfo.MinioPath, uploadID, completeParts, minio.PutObjectOptions{})
	if err != nil {
		slog.Error("CompleteMultipartUpload", "transfer_info", transferInfo, "err", err)
		return
	}

	slog.Info("transfer CompleteMultipartUpload", "transfer_info", transferInfo)
}

func (l Transfer) Loop() {
	go func() {
		for {
			select {
			case transferInfo := <-transferChan:
				err := transferPool.Invoke(transferInfo)
				if err != nil {
					slog.Error("transferPool Invoke", "err", err, "transferInfo", transferInfo)
					return
				}
			}
		}
	}()
}
