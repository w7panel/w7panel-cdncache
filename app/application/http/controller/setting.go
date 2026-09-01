package controller

import (
	"strings"

	"gitee.com/we7coreteam/w7-cdn-cache/app/application/logic"
	"github.com/gin-gonic/gin"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
)

type Setting struct {
	controller.Abstract
}

const globalSettingGroup = "global"

func (c Setting) List(ctx *gin.Context) {
	list, err := logic.Setting{}.StorageCacheList()
	if err != nil {
		c.JsonResponseWithServerError(ctx, err)
		return
	}

	mergeList := make(map[string]logic.StorageCacheSetting)
	for key, val := range list {
		if key == globalSettingGroup || val.Parent != "" {
			continue
		}
		tmpKey := key
		for key1, val1 := range list {
			if val1.Parent != "" && val1.Parent == key {
				tmpKey += "," + key1
			}
		}
		mergeList[tmpKey] = val
	}

	c.JsonResponseWithoutError(ctx, mergeList)
}

func (c Setting) Set(ctx *gin.Context) {
	type ParamsValidate struct {
		Host             string                   `json:"group" binding:"required"`
		StorageSource    logic.StorageSource      `json:"storage_source"`
		S3               logic.StorageS3          `json:"minio"`
		PathCacheRule    []logic.PathCacheRule    `json:"path_cache_rules"`
		PathKeyCacheRule []logic.PathKeyCacheRule `json:"path_key_cache_rules"`
		Extra            map[string]interface{}   `json:"extra"`
	}
	params := ParamsValidate{}
	if !c.Validate(ctx, &params) {
		return
	}

	host := strings.Split(params.Host, ",")
	parent := ""
	for i, item := range host {
		err := logic.Setting{}.SetStorageCacheSetting(item, logic.StorageCacheSetting{
			StorageSource:     &params.StorageSource,
			StorageCacheS3:    &params.S3,
			PathCacheRules:    params.PathCacheRule,
			PathKeyCacheRules: params.PathKeyCacheRule,
			Extra:             params.Extra,
			Parent:            parent,
		})
		if err != nil {
			c.JsonResponseWithServerError(ctx, err)
			return
		}
		if i == 0 {
			parent = item
		}
	}

	c.JsonSuccessResponse(ctx)
}

func (c Setting) Get(ctx *gin.Context) {
	type ParamsValidate struct {
		Host string `json:"group" form:"group" binding:"required"`
	}
	params := ParamsValidate{}
	if !c.Validate(ctx, &params) {
		return
	}

	host := strings.Split(params.Host, ",")
	setting := logic.Setting{}.GetStorageCacheSettingByHost(host[0])
	c.JsonResponseWithoutError(ctx, setting)
}

func (c Setting) Del(ctx *gin.Context) {
	type ParamsValidate struct {
		Host string `json:"group" form:"group" binding:"required"`
	}
	params := ParamsValidate{}
	if !c.Validate(ctx, &params) {
		return
	}

	host := strings.Split(params.Host, ",")
	for _, item := range host {
		logic.Setting{}.DelStorageCacheSetting(item)
	}

	c.JsonSuccessResponse(ctx)
}
