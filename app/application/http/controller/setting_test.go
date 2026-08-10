package controller

import (
	"encoding/json"
	"strings"
	"testing"

	"gitee.com/we7coreteam/w7-cdn-cache/app/application/logic"
)

func TestBuildCommonStorageCacheListFiltersSensitiveFields(t *testing.T) {
	list := map[string]logic.StorageCacheSetting{
		"global": {
			Extra: map[string]interface{}{
				"page_setting": map[string]interface{}{
					"markdown":       "# Public home",
					"copyright":      "Public copyright",
					"internal_token": "page-secret",
				},
				"cache_repository": map[string]interface{}{
					"username": "global-repository-user",
					"password": "global-repository-secret",
				},
			},
		},
		"mirror.example.com": {
			StorageSource: &logic.StorageSource{
				Endpoint:     "https://origin-user:origin-pass@origin.example.com/v2?token=source-secret#fragment",
				EndpointHost: "internal.example.com",
			},
			StorageCacheMinio: &logic.StorageMinio{
				AccessKey: "minio-access-key",
				SecretKey: "minio-secret-key",
			},
			Extra: map[string]interface{}{
				"cache_repository": map[string]interface{}{
					"username": "repository-user",
					"password": "repository-secret",
				},
			},
		},
	}

	result := buildCommonStorageCacheList(list)
	if result["global"].Extra == nil || result["global"].Extra.PageSetting == nil {
		t.Fatal("global page settings must be included in the public list")
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal common list: %v", err)
	}
	response := string(encoded)
	if !strings.Contains(response, "https://origin.example.com/v2") {
		t.Fatalf("sanitized origin is missing from response: %s", response)
	}
	if !strings.Contains(response, "# Public home") {
		t.Fatalf("global page settings are missing from response: %s", response)
	}
	for _, sensitive := range []string{
		"page-secret",
		"global-repository-user",
		"global-repository-secret",
		"origin-user",
		"origin-pass",
		"source-secret",
		"internal.example.com",
		"minio-access-key",
		"minio-secret-key",
		"repository-user",
		"repository-secret",
	} {
		if strings.Contains(response, sensitive) {
			t.Fatalf("public response contains sensitive value %q: %s", sensitive, response)
		}
	}
}

func TestSanitizeCommonEndpoint(t *testing.T) {
	value := "http://user:pass@one.example.com/a?token=secret, https://two.example.com/b#internal,invalid"
	want := "http://one.example.com/a,https://two.example.com/b"
	if got := sanitizeCommonEndpoint(value); got != want {
		t.Fatalf("sanitizeCommonEndpoint() = %q, want %q", got, want)
	}
}
