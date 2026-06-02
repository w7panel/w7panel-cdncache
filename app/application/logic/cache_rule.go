package logic

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

type PathCacheRule struct {
	CacheType    string   `json:"cache_type"`
	Paths        []string `json:"paths"`
	Enable       bool     `json:"enable"`
	CacheTtl     int64    `json:"cache_ttl"`
	Weight       int64    `json:"weight"`
	EnableStream bool     `json:"enable_stream"`
}

type PathKeyCacheRule struct {
	CacheType     string   `json:"cache_type"`
	Paths         []string `json:"paths"`
	IgnoreKeyRule string   `json:"ignore_key_rule"`
	Keys          []string `json:"keys"`
	IgnoreCase    bool     `json:"ignore_case"`
	Weight        int64    `json:"weight"`
}

type CacheRule struct {
	logic
}

func (l CacheRule) MatchPathCacheRule(path string, rules []PathCacheRule) (*PathCacheRule, error) {
	parsedURL, err := url.Parse(path)
	if err != nil {
		return nil, err
	}
	path = strings.TrimLeft(parsedURL.Path, "/")
	if rules == nil || len(rules) == 0 {
		return nil, nil
	}

	var defaultRule PathCacheRule
	for _, rule := range rules {
		switch rule.CacheType {
		case "suffix":
			for _, rpath := range rule.Paths {
				if strings.HasSuffix(path, rpath) {
					return &rule, nil
				}
			}
		case "path":
			for _, rpath := range rule.Paths {
				if path == rpath {
					return &rule, nil
				}
			}
		case "dir":
			for _, rpath := range rule.Paths {
				if strings.HasPrefix(path, rpath) {
					return &rule, nil
				}
			}
		case "all":
			defaultRule = rule // 保存匹配所有文件的规则
		default:
			fmt.Printf("Unknown cacheType: %s\n", rule.CacheType)
			return nil, fmt.Errorf("Unknown cacheType: %s", rule.CacheType)
		}
	}

	// 如果没有找到其他匹配规则，返回默认规则
	return &defaultRule, nil
}

func (l CacheRule) MatchPathKeyCacheRule(path string, rules []PathKeyCacheRule) (*PathKeyCacheRule, error) {
	parsedURL, err := url.Parse(path)
	if err != nil {
		return nil, err
	}
	path = strings.TrimLeft(parsedURL.Path, "/")
	if rules == nil || len(rules) == 0 {
		return nil, nil
	}

	var defaultRule PathKeyCacheRule
	for _, rule := range rules {
		switch rule.CacheType {
		case "suffix":
			if rule.IgnoreCase {
				for _, rpath := range rule.Paths {
					if strings.HasSuffix(strings.ToLower(path), strings.ToLower(rpath)) {
						return &rule, nil
					}
				}

			} else {
				for _, rpath := range rule.Paths {
					if strings.HasSuffix(path, rpath) {
						return &rule, nil
					}
				}
			}
		case "path":
			if rule.IgnoreCase {
				for _, rpath := range rule.Paths {
					if strings.ToLower(path) == strings.ToLower(rpath) {
						return &rule, nil
					}
				}
			} else {
				for _, rpath := range rule.Paths {
					if path == rpath {
						return &rule, nil
					}
				}
			}
		case "dir":
			if rule.IgnoreCase {
				for _, rpath := range rule.Paths {
					if strings.HasPrefix(strings.ToLower(path), strings.ToLower(rpath)) {
						return &rule, nil
					}
				}
			} else {
				for _, rpath := range rule.Paths {
					if strings.HasPrefix(path, rpath) {
						return &rule, nil
					}
				}
			}
		case "all":
			defaultRule = rule // 保存匹配所有文件的规则
		default:
			fmt.Printf("Unknown cacheType: %s\n", rule.CacheType)
			return nil, fmt.Errorf("Unknown cacheType: %s", rule.CacheType)
		}
	}

	// 如果没有找到其他匹配规则，返回默认规则
	return &defaultRule, nil
}

func (l CacheRule) ReBuildPathByRule(path string, rule *PathKeyCacheRule) string {
	parsedURL, err := url.Parse(path)
	if err != nil {
		return path
	}

	// 根据 IgnoreKeyRule 处理查询参数
	switch rule.IgnoreKeyRule {
	case "ignore":
		parsedURL.RawQuery = ""
	case "keep":
		parsedURL.RawQuery = parsedURL.Query().Encode()
	case "keep_specified":
		query := url.Values{}
		for _, key := range rule.Keys {
			if parsedURL.Query().Has(key) {
				query.Add(key, parsedURL.Query().Get(key))
			}
		}
		parsedURL.RawQuery = query.Encode()
	case "ignore_specified":
		query := url.Values{}
		for key, values := range parsedURL.Query() {
			for _, value := range values {
				query.Add(key, value)
			}
		}
		for _, key := range rule.Keys {
			query.Del(key)
		}
		parsedURL.RawQuery = query.Encode()
	}

	return parsedURL.String()
}

func (l CacheRule) GetPathCacheSavePath(originPath string) string {
	query := ""
	file := originPath
	if idx := strings.Index(originPath, "?"); idx != -1 {
		query = originPath[idx+1:]
		file = originPath[:idx]
	}

	if query != "" {
		// 计算 MD5
		hash := md5.Sum([]byte(query))
		md5Hash := hex.EncodeToString(hash[:])

		// 分离文件名和扩展名
		ext := filepath.Ext(file)
		base := strings.TrimSuffix(file, ext)

		// 构建新文件名
		return fmt.Sprintf("%s-%s%s", base, md5Hash, ext)
	}

	return originPath
}
