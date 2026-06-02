package logic

import (
	"sync"

	"gitee.com/we7coreteam/w7-cdn-cache/common/helper"
)

var defaultStorageLoadBalanceMap = sync.Map{}

type LoadBalance struct {
}

func (l LoadBalance) Reset(host string) {
	defaultStorageLoadBalanceMap.Delete(host)
}

func (l LoadBalance) GetBackend(host string) *helper.Backend {
	setting := Setting{}.GetStorageCacheSettingByHost(host)
	if len(setting.StorageSource.Endpoints) == 1 {
		return &helper.Backend{
			URL: setting.StorageSource.Endpoints[0],
		}
	}

	var loadBalance *helper.LoadBalancer
	val, exists := defaultStorageLoadBalanceMap.Load(host)
	if !exists {
		loadBalance = helper.NewLoadBalancer(setting.StorageSource.Endpoints, 100000, 0.1)
		defaultStorageLoadBalanceMap.Store(host, loadBalance)
	} else {
		loadBalance = val.(*helper.LoadBalancer)
	}

	return loadBalance.GetBackend()
}
