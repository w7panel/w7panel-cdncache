package logic

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
)

var settingFileSuffix = "-setting.json"

const DefaultPathPrefix = "/"

type StorageMinio struct {
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
	Bucket    string `json:"bucket"`
	Endpoint  string `json:"endpoint"`
	Region    string `json:"region"`
}

type StorageSource struct {
	Endpoint     string   `json:"endpoint"`
	EndpointHost string   `json:"endpoint_host"`
	Endpoints    []string `json:"-"`
}

type StorageCacheSetting struct {
	StorageSource     *StorageSource         `json:"storage_source"`
	StorageCacheMinio *StorageMinio          `json:"minio"`
	PathCacheRules    []PathCacheRule        `json:"path_cache_rules"`
	PathKeyCacheRules []PathKeyCacheRule     `json:"path_key_cache_rules"`
	PurgeReqMethod    string                 `json:"purge_req_method"`
	Extra             map[string]interface{} `json:"extra"`
	Parent            string                 `json:"parent"`
}

var defaultStorageSettingMap = sync.Map{}

type Setting struct {
	logic
}

func (l Setting) SetStorageCacheSetting(host string, cacheSetting StorageCacheSetting) error {
	cacheSettingMap := l.GetStorageCacheSettingMap(host)

	if cacheSetting.PathCacheRules != nil {
		sort.Slice(cacheSetting.PathCacheRules, func(i, j int) bool {
			return cacheSetting.PathCacheRules[i].Weight < cacheSetting.PathCacheRules[j].Weight
		})
	}
	if cacheSetting.PathKeyCacheRules != nil {
		sort.Slice(cacheSetting.PathKeyCacheRules, func(i, j int) bool {
			return cacheSetting.PathKeyCacheRules[i].Weight < cacheSetting.PathKeyCacheRules[j].Weight
		})
	}
	cacheSetting.PurgeReqMethod = strings.ToLower(cacheSetting.PurgeReqMethod)

	cacheSettingMap[DefaultPathPrefix] = cacheSetting

	settingContent, err := json.Marshal(cacheSettingMap)
	if err != nil {
		return err
	}

	settingSaveDir := filepath.Dir(facade.GetConfig().GetString("database.default.db_name"))
	err = os.MkdirAll(settingSaveDir, 0755)
	settingSavePath := filepath.Join(settingSaveDir, host+settingFileSuffix)
	err = os.WriteFile(settingSavePath, settingContent, 0644)
	if err != nil {
		return err
	}

	if cacheSetting.StorageCacheMinio != nil && cacheSetting.StorageCacheMinio.Endpoint != "" {
		err := S3Client{}.ResetMinioClient(host, *cacheSetting.StorageCacheMinio)
		if err != nil {
			return err
		}
	}

	defaultStorageSettingMap.Delete(host)
	LoadBalance{}.Reset(host)

	return nil
}

func (l Setting) GetStorageCacheSettingMap(host string) map[string]StorageCacheSetting {
	cacheSettingMap := make(map[string]StorageCacheSetting)
	val, exists := defaultStorageSettingMap.Load(host)
	if !exists {
		settingSaveDir := filepath.Dir(facade.GetConfig().GetString("database.default.db_name"))
		settingSavePath := filepath.Join(settingSaveDir, host+settingFileSuffix)
		if _, err := os.Stat(settingSavePath); os.IsNotExist(err) {
			return cacheSettingMap
		}

		content, err := os.ReadFile(settingSavePath)
		if err != nil {
			slog.Error("GetStorageCacheSetting: os.ReadFile(settingSavePath) error", "err", err)
			return cacheSettingMap
		}
		err = json.Unmarshal(content, &cacheSettingMap)
		if err != nil {
			slog.Error("GetStorageCacheSetting: json.Unmarshal() error", "err", err)
			return cacheSettingMap
		}

		for key, item := range cacheSettingMap {
			if item.StorageSource != nil {
				item.StorageSource.Endpoints = strings.Split(item.StorageSource.Endpoint, ",")
			}
			if item.StorageCacheMinio != nil && item.StorageCacheMinio.Endpoint != "" {
				err = S3Client{}.ResetMinioClient(host, *item.StorageCacheMinio)
				if err != nil {
					slog.Error("GetStorageCacheSetting: ResetStorageMinioClient() error", "err", err)
				}
			}

			cacheSettingMap[key] = item
		}

		defaultStorageSettingMap.Store(host, cacheSettingMap)
	} else {
		cacheSettingMap = val.(map[string]StorageCacheSetting)
	}

	return cacheSettingMap
}

func (l Setting) GetStorageCacheSettingByHost(host string) StorageCacheSetting {
	cacheSettingMap := l.GetStorageCacheSettingMap(host)
	if cacheSetting, ok := cacheSettingMap[DefaultPathPrefix]; ok {
		return cacheSetting
	}
	return StorageCacheSetting{}
}

func (l Setting) DelStorageCacheSetting(host string) {
	settingSaveDir := filepath.Dir(facade.GetConfig().GetString("database.default.db_name"))
	settingSavePath := filepath.Join(settingSaveDir, host+settingFileSuffix)
	os.Remove(settingSavePath)
	defaultStorageSettingMap.Delete(host)
	LoadBalance{}.Reset(host)
}

func (l Setting) StorageCacheList() (map[string]StorageCacheSetting, error) {
	settingSaveDir := filepath.Dir(facade.GetConfig().GetString("database.default.db_name"))

	entries, err := os.ReadDir(settingSaveDir)
	if err != nil {
		return nil, err
	}

	list := make(map[string]StorageCacheSetting)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if !strings.HasSuffix(entry.Name(), settingFileSuffix) {
			continue
		}

		host := strings.TrimSuffix(entry.Name(), settingFileSuffix)
		if host == "" {
			continue
		}
		list[host] = l.GetStorageCacheSettingByHost(host)
	}
	return list, nil
}
