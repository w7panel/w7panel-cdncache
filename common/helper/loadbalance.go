package helper

import (
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// 负载均衡器
type LoadBalancer struct {
	backends       []*Backend
	mu             sync.Mutex // 锁
	minSuccessRate float64    // 最低成功率阈值
}

func NewLoadBalancer(targetUrls []string, maxReqNum int64, minSuccessRate float64) *LoadBalancer {
	lb := &LoadBalancer{
		minSuccessRate: minSuccessRate,
	}

	for _, url := range targetUrls {
		backend := &Backend{
			URL:       url,
			MaxReqNum: maxReqNum,
		}
		lb.backends = append(lb.backends, backend)
	}

	return lb
}

// 基于成功率的负载均衡策略
func (lb *LoadBalancer) GetBackend() *Backend {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	// 计算所有后端的权重总和
	totalWeight := 0.0
	var aliveBackends []*Backend

	for _, backend := range lb.backends {
		aliveBackends = append(aliveBackends, backend)

		// 计算当前权重
		successRate := backend.GetSuccessRate()
		weight := lb.calculateWeight(successRate)
		totalWeight += weight
	}

	// 随机选择（基于权重）
	randWeight := rand.Float64() * totalWeight
	currentWeight := 0.0

	for _, backend := range aliveBackends {
		successRate := backend.GetSuccessRate()
		weight := lb.calculateWeight(successRate)

		currentWeight += weight
		if randWeight < currentWeight {
			return backend
		}
	}

	// 不应该执行到这里
	return aliveBackends[0]
}

// 计算权重
func (lb *LoadBalancer) calculateWeight(successRate float64) float64 {
	// 确保成功率不低于最低阈值
	adjustedRate := math.Max(successRate, lb.minSuccessRate)

	// 使用指数函数平滑权重
	// 当成功率接近1时权重增长更快
	return math.Pow(adjustedRate, 2) * 100
}

type RequestRecord struct {
	success bool
	time    time.Time
}

// 后端服务器配置
type Backend struct {
	URL          string
	SuccessCount atomic.Int64 // 成功请求计数
	TotalCount   atomic.Int64 // 总请求计数
	MaxReqNum    int64
}

func (b *Backend) GetSuccessRate() float64 {
	if b.TotalCount.Load() == 0 {
		return 1.0 // 没有请求时视为100%成功率
	}

	return min(float64(b.SuccessCount.Load())/float64(b.TotalCount.Load()), 1.0)
}

func (b *Backend) changeSuccessCount(changeNum int64) {
	successCount := b.SuccessCount.Load()
	if successCount > b.MaxReqNum {
		successCount -= b.MaxReqNum
		b.SuccessCount.Store(successCount)
	} else {
		b.SuccessCount.Add(changeNum)
	}
}

func (b *Backend) changeTotalCount(changeNum int64) {
	totalCount := b.TotalCount.Load()
	if totalCount > b.MaxReqNum {
		totalCount -= b.MaxReqNum
		successCount := b.SuccessCount.Load()
		if totalCount < successCount {
			totalCount = successCount
		}
		b.TotalCount.Store(totalCount)
	} else {
		b.TotalCount.Add(changeNum)
	}
}

func (b *Backend) RecordRequest(success bool) {
	b.changeTotalCount(1)
	if success {
		b.changeSuccessCount(1)
	} else {
		b.changeSuccessCount(-1)
	}
}
