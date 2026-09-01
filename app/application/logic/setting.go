package logic

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
)

var settingFileSuffix = "-setting.json"

const DefaultPathPrefix = "/"

const globalSettingGroup = "global"

type StorageS3 struct {
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
	StorageCacheS3    *StorageS3             `json:"minio"`
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

// normalizeS3Endpoint validates and canonicalizes an endpoint before it is
// persisted. Bare host:port values retain the legacy HTTP behavior; HTTPS must
// be specified explicitly.
func normalizeS3Endpoint(endpoint string) (string, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", fmt.Errorf("s3 endpoint is required")
	}
	if !strings.Contains(endpoint, "://") {
		endpoint = "http://" + endpoint
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("invalid s3 endpoint: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" {
		return "", fmt.Errorf("invalid s3 endpoint %q", endpoint)
	}
	return strings.TrimRight(endpoint, "/"), nil
}

func (l Setting) SetStorageCacheSetting(host string, cacheSetting StorageCacheSetting) error {
	if cacheSetting.StorageCacheS3 != nil && cacheSetting.StorageCacheS3.Endpoint != "" {
		normalizedEndpoint, err := normalizeS3Endpoint(cacheSetting.StorageCacheS3.Endpoint)
		if err != nil {
			return fmt.Errorf("invalid S3 endpoint: %w", err)
		}
		cacheSetting.StorageCacheS3.Endpoint = normalizedEndpoint
	}
	cacheSettingMap := l.GetStorageCacheSettingMap(host)
	if host != globalSettingGroup && storageConfigMode(cacheSetting.Extra) == "global" {
		// 全局模式继承连接配置，但允许站点使用独立的 bucket。
		bucket := ""
		if cacheSetting.StorageCacheS3 != nil {
			bucket = cacheSetting.StorageCacheS3.Bucket
		}
		cacheSetting.StorageCacheS3 = &StorageS3{Bucket: bucket}
	}

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

	nextCacheSettingMap := cloneStorageSettingMap(cacheSettingMap)
	nextCacheSettingMap.Store(DefaultPathPrefix, cacheSetting)

	settingContent, err := json.Marshal(storageSettingMapSnapshot(nextCacheSettingMap))
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

	if cacheSetting.StorageCacheS3 != nil && cacheSetting.StorageCacheS3.Endpoint != "" {
		err := S3Client{}.ResetClient(host, *cacheSetting.StorageCacheS3)
		if err != nil {
			return err
		}
	}

	// 配置保存成功后使内存缓存失效，下一次读取时从文件重新加载并补齐派生字段（如 Endpoints）。
	defaultStorageSettingMap.Delete(host)
	LoadBalance{}.Reset(host)

	return nil
}

func (l Setting) GetStorageCacheSettingMap(host string) *sync.Map {
	val, exists := defaultStorageSettingMap.Load(host)
	if !exists {
		cacheSettingMap := make(map[string]StorageCacheSetting)
		settingSaveDir := filepath.Dir(facade.GetConfig().GetString("database.default.db_name"))
		settingSavePath := filepath.Join(settingSaveDir, host+settingFileSuffix)
		if _, err := os.Stat(settingSavePath); os.IsNotExist(err) {
			return storageSettingMapFromMap(cacheSettingMap)
		}

		content, err := os.ReadFile(settingSavePath)
		if err != nil {
			slog.Error("GetStorageCacheSetting: os.ReadFile(settingSavePath) error", "err", err)
			return storageSettingMapFromMap(cacheSettingMap)
		}
		err = json.Unmarshal(content, &cacheSettingMap)
		if err != nil {
			slog.Error("GetStorageCacheSetting: json.Unmarshal() error", "err", err)
			return storageSettingMapFromMap(cacheSettingMap)
		}

		for key, item := range cacheSettingMap {
			if item.StorageSource != nil {
				item.StorageSource.Endpoints = strings.Split(item.StorageSource.Endpoint, ",")
			}
			if item.StorageCacheS3 != nil && item.StorageCacheS3.Endpoint != "" {
				err = S3Client{}.ResetClient(host, *item.StorageCacheS3)
				if err != nil {
					slog.Error("GetStorageCacheSetting: ResetS3Client() error", "err", err)
				}
			}

			cacheSettingMap[key] = item
		}

		loadedCacheSettingMap := storageSettingMapFromMap(cacheSettingMap)
		actual, _ := defaultStorageSettingMap.LoadOrStore(host, loadedCacheSettingMap)
		return actual.(*sync.Map)
	}

	return val.(*sync.Map)
}

func (l Setting) GetStorageCacheSettingByHost(host string) StorageCacheSetting {
	cacheSettingMap := l.GetStorageCacheSettingMap(host)
	if value, ok := cacheSettingMap.Load(DefaultPathPrefix); ok {
		cacheSetting, ok := value.(StorageCacheSetting)
		if !ok {
			return StorageCacheSetting{}
		}
		if host != globalSettingGroup && storageConfigMode(cacheSetting.Extra) == "global" {
			globalSetting := l.GetStorageCacheSettingByHost(globalSettingGroup)
			if globalSetting.StorageCacheS3 != nil {
				globalStorage := resolveGlobalStorage(
					*globalSetting.StorageCacheS3,
					cacheSetting.StorageCacheS3,
				)
				cacheSetting.StorageCacheS3 = &globalStorage
				if globalStorage.Endpoint != "" {
					if err := (S3Client{}).ResetClient(host, globalStorage); err != nil {
						slog.Error("GetStorageCacheSettingByHost: ResetS3Client() error", "err", err)
					}
				}
			}
		}
		return cacheSetting
	}
	return StorageCacheSetting{}
}

func storageSettingMapFromMap(settings map[string]StorageCacheSetting) *sync.Map {
	result := &sync.Map{}
	for key, setting := range settings {
		result.Store(key, setting)
	}
	return result
}

func storageSettingMapSnapshot(settings *sync.Map) map[string]StorageCacheSetting {
	result := make(map[string]StorageCacheSetting)
	if settings == nil {
		return result
	}
	settings.Range(func(key, value interface{}) bool {
		keyString, ok := key.(string)
		if !ok {
			return true
		}
		setting, ok := value.(StorageCacheSetting)
		if ok {
			result[keyString] = setting
		}
		return true
	})
	return result
}

func cloneStorageSettingMap(settings *sync.Map) *sync.Map {
	return storageSettingMapFromMap(storageSettingMapSnapshot(settings))
}

func resolveGlobalStorage(globalStorage StorageS3, siteStorage *StorageS3) StorageS3 {
	if siteStorage != nil && siteStorage.Bucket != "" {
		globalStorage.Bucket = siteStorage.Bucket
	}
	return globalStorage
}

func storageConfigMode(extra map[string]interface{}) string {
	storageConfig, ok := extra["storage_config"].(map[string]interface{})
	if !ok {
		return ""
	}
	mode, _ := storageConfig["mode"].(string)
	return mode
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
