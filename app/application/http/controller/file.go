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
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/gin-gonic/gin"
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
	if setting.StorageSource == nil || setting.StorageSource.Endpoint == "" || setting.StorageCacheS3 == nil || setting.StorageCacheS3.Bucket == "" {
		c.JsonResponseWithServerError(ctx, errors.New("附件源配置错误"))
		return
	}
	s3Client := logic.S3Client{}.GetClient(params.Host)
	if s3Client == nil {
		c.JsonResponseWithServerError(ctx, errors.New("附件缓存存储配置错误"))
		return
	}
	paginator := s3.NewListObjectsV2Paginator(s3Client, &s3.ListObjectsV2Input{
		Bucket: aws.String(setting.StorageCacheS3.Bucket),
		Prefix: aws.String(params.Path),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			c.JsonResponseWithServerError(ctx, err)
			return
		}
		objects := make([]types.ObjectIdentifier, 0, len(page.Contents))
		for _, object := range page.Contents {
			if object.Key != nil {
				objects = append(objects, types.ObjectIdentifier{Key: object.Key})
			}
		}
		if len(objects) == 0 {
			continue
		}
		deleteOutput, err := s3Client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(setting.StorageCacheS3.Bucket),
			Delete: &types.Delete{Objects: objects},
		})
		if err != nil {
			c.JsonResponseWithServerError(ctx, err)
			return
		}
		if deleteOutput == nil {
			c.JsonResponseWithServerError(ctx, errors.New("S3 删除返回空响应"))
			return
		}
		if len(deleteOutput.Errors) > 0 {
			item := deleteOutput.Errors[0]
			c.JsonResponseWithServerError(ctx, fmt.Errorf("delete object %q failed: %s: %s", aws.ToString(item.Key), aws.ToString(item.Code), aws.ToString(item.Message)))
			return
		}
	}

	c.JsonSuccessResponse(ctx)
}

func (c File) Download(ctx *gin.Context) {
	host := ctx.Request.Host
	reqPath := ctx.Request.RequestURI
	setting := logic.Setting{}.GetStorageCacheSettingByHost(host)
	if setting.StorageSource == nil || setting.StorageSource.Endpoint == "" || setting.StorageCacheS3 == nil || setting.StorageCacheS3.Bucket == "" {
		c.JsonResponseWithServerError(ctx, errors.New("附件源配置错误"))
		return
	}
	s3Client := logic.S3Client{}.GetClient(host)
	if s3Client == nil {
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
	s3ExistsCache := false
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
		s3ObjectInfo, err := logic.Storage{}.GetObjectInfoByS3(ctx, s3Client, setting.StorageCacheS3.Bucket, cacheSavePath)
		slog.Info("HeadObject with s3", "path", cacheSavePath, "info", s3ObjectInfo, "err", err)
		if err == nil {
			existsCache = true
			s3ExistsCache = true
			if cacheTtl > 0 && time.Since(s3ObjectInfo.LastModified).Minutes() > float64(cacheTtl) {
				existsCache = false
			}
			if existsCache {
				modifySince := ctx.Request.Header.Get("If-Modified-Since")
				if modifySince != "" {
					clientTime, _ := http.ParseTime(modifySince)
					if !clientTime.IsZero() && s3ObjectInfo.LastModified.Before(clientTime) {
						ctx.Status(http.StatusNotModified)
						return
					}
				}
			}
			resourcesSize = s3ObjectInfo.Size
			downloadChunkFunc = logic.Storage{}.DownloadChunkByS3(s3Client, setting.StorageCacheS3.Bucket)
			downloadPath = cacheSavePath
			downloadHeader = s3ObjectInfo.Header
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
		} else if s3ExistsCache {
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
			S3Path:        cacheSavePath,
			CacheTTL:      cacheTtl,
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
