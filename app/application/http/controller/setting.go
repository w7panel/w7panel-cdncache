package controller

import (
	"encoding/json"
	"net/url"
	"strings"

	"gitee.com/we7coreteam/w7-cdn-cache/app/application/logic"
	"github.com/gin-gonic/gin"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
)

type Setting struct {
	controller.Abstract
}

type commonStorageSource struct {
	Endpoint string `json:"endpoint"`
}

type commonPageSetting struct {
	Markdown      string `json:"markdown"`
	ICPNumber     string `json:"icp_number"`
	ICPLink       string `json:"icp_link"`
	PoliceNumber  string `json:"police_number"`
	PoliceLink    string `json:"police_link"`
	Copyright     string `json:"copyright"`
	CopyrightLink string `json:"copyright_link"`
}

type commonExtra struct {
	PageSetting *commonPageSetting `json:"page_setting,omitempty"`
}

type commonStorageCacheSetting struct {
	StorageSource *commonStorageSource `json:"storage_source,omitempty"`
	Extra         *commonExtra         `json:"extra,omitempty"`
}

func mergeStorageCacheList() (map[string]logic.StorageCacheSetting, error) {
	list, err := logic.Setting{}.StorageCacheList()
	if err != nil {
		return nil, err
	}

	mergeList := make(map[string]logic.StorageCacheSetting)
	for key, val := range list {
		if val.Parent != "" {
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
	return mergeList, nil
}

func (c Setting) List(ctx *gin.Context) {
	mergeList, err := mergeStorageCacheList()
	if err != nil {
		c.JsonResponseWithServerError(ctx, err)
		return
	}

	c.JsonResponseWithoutError(ctx, mergeList)
}

func (c Setting) CommonList(ctx *gin.Context) {
	list, err := mergeStorageCacheList()
	if err != nil {
		c.JsonResponseWithServerError(ctx, err)
		return
	}

	c.JsonResponseWithoutError(ctx, buildCommonStorageCacheList(list))
}

func buildCommonStorageCacheList(list map[string]logic.StorageCacheSetting) map[string]commonStorageCacheSetting {
	// 公开接口使用字段白名单，避免对象存储密钥、仓库凭据、缓存规则及
	// 后续新增的内部字段被意外暴露。
	commonList := make(map[string]commonStorageCacheSetting)
	for group, setting := range list {
		if group == "global" {
			commonList[group] = commonStorageCacheSetting{
				Extra: &commonExtra{
					PageSetting: commonPageSettingFromExtra(setting.Extra),
				},
			}
			continue
		}

		commonSetting := commonStorageCacheSetting{}
		if setting.StorageSource != nil {
			commonSetting.StorageSource = &commonStorageSource{
				Endpoint: sanitizeCommonEndpoint(setting.StorageSource.Endpoint),
			}
		}
		commonList[group] = commonSetting
	}
	return commonList
}

func commonPageSettingFromExtra(extra map[string]interface{}) *commonPageSetting {
	value, exists := extra["page_setting"]
	if !exists {
		return nil
	}

	content, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	pageSetting := commonPageSetting{}
	if err = json.Unmarshal(content, &pageSetting); err != nil {
		return nil
	}
	return &pageSetting
}

func sanitizeCommonEndpoint(value string) string {
	endpoints := strings.Split(value, ",")
	sanitized := make([]string, 0, len(endpoints))
	for _, endpoint := range endpoints {
		parsed, err := url.Parse(strings.TrimSpace(endpoint))
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			continue
		}

		parsed.User = nil
		parsed.RawQuery = ""
		parsed.ForceQuery = false
		parsed.Fragment = ""
		sanitized = append(sanitized, parsed.String())
	}
	return strings.Join(sanitized, ",")
}

func (c Setting) Set(ctx *gin.Context) {
	type ParamsValidate struct {
		Host             string                   `json:"group" binding:"required"`
		StorageSource    logic.StorageSource      `json:"storage_source"`
		Minio            logic.StorageMinio       `json:"minio"`
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
			StorageCacheMinio: &params.Minio,
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
