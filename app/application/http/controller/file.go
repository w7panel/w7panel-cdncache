package controller

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gitee.com/we7coreteam/w7-cdn-cache/app/application/logic"
	"gitee.com/we7coreteam/w7-cdn-cache/common/helper"
	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
)

type File struct {
	controller.Abstract
}

func (c File) ClearFileCache(ctx *gin.Context) {
	type ParamsValidate struct {
		Host string `json:"group" binding:"required"`
		Path string `json:"path" binding:"required"`
	}
	params := ParamsValidate{}
	if !c.Validate(ctx, &params) {
		return
	}
	params.Path = "/" + strings.TrimPrefix(params.Path, "/")
	params.Host = strings.TrimRight(params.Host, "/")

	if params.Path == "/" {
		c.JsonResponseWithServerError(ctx, errors.New("请填写正确的路径"))
		return
	}

	setting := logic.Setting{}.GetStorageCacheSettingByHost(params.Host)
	if setting.StorageSource == nil || setting.StorageSource.Endpoint == "" {
		c.JsonResponseWithServerError(ctx, errors.New("附件源配置错误"))
		return
	}
	minioClient := logic.S3Client{}.GetMinioClient(params.Host)
	if minioClient == nil {
		c.JsonResponseWithServerError(ctx, errors.New("附件缓存存储配置错误"))
		return
	}
	params.Path = strings.TrimPrefix(params.Path, "/")

	objectsCh := make(chan minio.ObjectInfo)
	go func() {
		defer close(objectsCh)
		for object := range minioClient.ListObjectsIter(ctx, setting.StorageCacheMinio.Bucket, minio.ListObjectsOptions{Prefix: params.Path, Recursive: true}) {
			objectsCh <- object
		}
	}()

	// 删除对象
	for rErr := range minioClient.RemoveObjects(ctx, setting.StorageCacheMinio.Bucket, objectsCh, minio.RemoveObjectsOptions{}) {
		if rErr.Err != nil {
			c.JsonResponseWithServerError(ctx, rErr.Err)
			return
		}
	}

	c.JsonSuccessResponse(ctx)
}

func (c File) Download(ctx *gin.Context) {
	host := ctx.Request.Host
	reqPath := ctx.Request.RequestURI
	setting := logic.Setting{}.GetStorageCacheSettingByHost(host)
	if setting.StorageSource == nil || setting.StorageSource.Endpoint == "" {
		c.JsonResponseWithServerError(ctx, errors.New("附件源配置错误"))
		return
	}
	minioClient := logic.S3Client{}.GetMinioClient(host)
	if minioClient == nil {
		c.JsonResponseWithServerError(ctx, errors.New("附件缓存存储配置错误"))
		return
	}
	if reqPath == "/" {
		c.JsonResponseWithServerError(ctx, errors.New("源文件不存在"))
		return
	}

	cacheRuleLogic := logic.CacheRule{}
	enableCache := false
	cacheTtl := int64(-1)
	minioExistsCache := false
	existsCache := false
	cacheSavePath := ""
	resourcesSize := int64(0)
	remoteUrl := ""
	var downloadChunkFunc logic.ChunkDownloadFunc
	downloadPath := ""
	downloadHeader := http.Header{}
	sourceHeaders := http.Header{}
	enableStream := false

	pathCacheRule, err := cacheRuleLogic.MatchPathCacheRule(reqPath, setting.PathCacheRules)
	slog.Info("match pathCacheRule", "path", reqPath, "match_rule", pathCacheRule, "err", err)
	if pathCacheRule != nil && pathCacheRule.Enable {
		enableCache = pathCacheRule.Enable
		cacheTtl = pathCacheRule.CacheTtl
		enableStream = pathCacheRule.EnableStream
		pathKeyCacheRule, err := cacheRuleLogic.MatchPathKeyCacheRule(reqPath, setting.PathKeyCacheRules)
		slog.Info("match pathKeyCacheRule", "path", reqPath, "match_rule", pathKeyCacheRule, "err", err)
		if pathKeyCacheRule != nil {
			reqPath = cacheRuleLogic.ReBuildPathByRule(reqPath, pathKeyCacheRule)
		}
		cacheSavePath = cacheRuleLogic.GetPathCacheSavePath(reqPath)
	}

	if enableCache && strings.ToLower(ctx.Request.Method) != setting.PurgeReqMethod {
		minioObjectInfo, err := logic.Storage{}.GetObjectInfoByMinio(ctx, minioClient, setting.StorageCacheMinio.Bucket, cacheSavePath)
		slog.Info("StatObject with minio", "path", cacheSavePath, "info", minioObjectInfo, "err", err)
		if err == nil {
			existsCache = true
			minioExistsCache = true
			if cacheTtl > 0 && time.Since(minioObjectInfo.LastModified).Minutes() > float64(cacheTtl) {
				existsCache = false
			}
			if existsCache {
				modifySince := ctx.Request.Header.Get("If-Modified-Since")
				if modifySince != "" {
					clientTime, _ := http.ParseTime(modifySince)
					if !clientTime.IsZero() && minioObjectInfo.LastModified.Before(clientTime) {
						ctx.Status(http.StatusNotModified)
						return
					}
				}
			}
			resourcesSize = minioObjectInfo.Size
			downloadChunkFunc = logic.Storage{}.DownloadChunkByMinio(minioClient, setting.StorageCacheMinio.Bucket)
			downloadPath = cacheSavePath
			downloadHeader = minioObjectInfo.Header
		}
	}
	var backend *helper.Backend
	backendReqSuccess := false
	defer func() {
		if backend != nil {
			backend.RecordRequest(backendReqSuccess)
		}
	}()
	if !existsCache {
		backend = logic.LoadBalance{}.GetBackend(host)
		remoteUrl = strings.TrimRight(backend.URL, "/") + "/" + strings.TrimLeft(reqPath, "/")
		for key, val := range ctx.Request.Header {
			if key == "If-Modified-Since" || key == "If-None-Match" || key == "Range" {
				continue
			}
			sourceHeaders.Add(key, val[0])
		}
		if setting.StorageSource.EndpointHost != "" {
			sourceHeaders.Set("Host", setting.StorageSource.EndpointHost)
		}
		httpObjectInfo, err := logic.Storage{}.GetObjectInfoByHttp(ctx, remoteUrl, sourceHeaders)
		slog.Info("StatObject with http", "path", reqPath, "info", httpObjectInfo, "err", err)
		if err == nil {
			resourcesSize = httpObjectInfo.Size
			downloadChunkFunc = logic.Storage{}.DownloadChunkByHttp()
			downloadPath = remoteUrl
			downloadHeader = httpObjectInfo.Header
		} else if minioExistsCache {
			existsCache = true
			backend = nil
		} else {
			c.JsonResponseWithServerError(ctx, errors.New("获取源文件信息失败"))
			return
		}
	}
	if resourcesSize == 0 {
		c.JsonResponseWithServerError(ctx, errors.New("文件大小异常"))
		return
	}

	for key, val := range downloadHeader {
		ctx.Header(key, val[0])
	}

	if cacheTtl > 0 {
		ctx.Header("Cache-Control", "public, max-age="+strconv.FormatInt(cacheTtl*60, 10)+", immutable")
	} else if cacheTtl == 0 {
		ctx.Header("Cache-Control", "public, max-age=315360000, immutable")
	}
	if ctx.Request.Method == http.MethodHead {
		ctx.Header("Transfer-Encoding", "")
		ctx.Header("Content-Length", strconv.FormatInt(resourcesSize, 10))
		ctx.Status(http.StatusOK)
		if backend != nil {
			backendReqSuccess = true
		}
		return
	}
	if enableStream {
		ctx.Header("Content-Length", "")
		ctx.Header("Transfer-Encoding", "chunked")
	}

	if enableCache && !existsCache {
		go logic.Transfer{}.Push(logic.TransferInfo{
			Host:          host,
			RemoteUrl:     remoteUrl,
			MinioPath:     cacheSavePath,
			ResourceSize:  resourcesSize,
			ContentType:   downloadHeader.Get("Content-Type"),
			CacheSetting:  setting,
			SourceHeaders: sourceHeaders,
		})
	}

	rangeHeader := ctx.GetHeader("Range")
	offset, end := int64(0), resourcesSize-1
	if rangeHeader != "" {
		if strings.HasPrefix(rangeHeader, "bytes=") {
			parts := strings.Split(rangeHeader[6:], "-")
			if len(parts) > 0 {
				offset, _ = strconv.ParseInt(parts[0], 10, 64)
				if len(parts) > 1 && parts[1] != "" {
					end, _ = strconv.ParseInt(parts[1], 10, 64)
				}
			}
			ctx.Status(http.StatusPartialContent)
			ctx.Header("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, end, resourcesSize))
			ctx.Header("Transfer-Encoding", "")
			ctx.Header("Content-Length", strconv.FormatInt(end-offset+1, 10))
		}
	} else {
		ctx.Status(http.StatusOK)
	}

	err = logic.Storage{}.DownloadChunk(ctx, downloadChunkFunc, downloadPath, offset, end, sourceHeaders, ctx.Writer)
	if backend != nil {
		backendReqSuccess = err == nil
	}
	if err != nil {
		c.JsonResponseWithServerError(ctx, err)
		return
	}
}
