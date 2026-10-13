package router

import (
	"sync"
	"time"

	"nova-erp/internal/database"
)

// healthTTL 是健康检查结果的缓存时长。
const healthTTL = time.Second

// /health 每次都要 Ping 一次数据库，而它是免鉴权接口 ——
// 探针密度过高或被人拿来刷接口时，等于给外部开了一条持续打库的通道。
// 这里做 1 秒记忆：可用性信息最多滞后 1 秒，但到数据库的探测频率
// 被压到每秒最多一次，与请求量无关。
type healthSnapshot struct {
	err      error
	expireAt time.Time
}

var (
	healthMu     sync.Mutex
	healthCached healthSnapshot
)

// healthError 返回缓存的数据库健康状态。
func healthError() error {
	now := time.Now()

	healthMu.Lock()
	defer healthMu.Unlock()
	if now.Before(healthCached.expireAt) {
		return healthCached.err
	}
	err := database.Health()
	healthCached = healthSnapshot{err: err, expireAt: now.Add(healthTTL)}
	return err
}
