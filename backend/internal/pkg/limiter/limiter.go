// Package limiter 提供一个极简的「按 key 统计失败次数」内存限流器，
// 用于认证接口的粗粒度防爆破。
package limiter

import (
	"sync"
	"time"
)

// FailLimiter 统计每个 key 在滑动时间窗内的失败次数，超过阈值即拒绝。
//
// 为什么刻意「只统计失败」而不是统计全部请求：
// 成功请求不消耗额度，因此共享出口 IP（公司网关 / NAT）下的正常用户
// 永远不会因为并发量大被误伤；只有真的在连续失败（撞库、爆破）的来源才会被限流。
// 这正好补上「按账号锁定」拦不住的场景 —— 用同一个 IP 去试大量不同账号。
//
// 限制：状态在进程内存里，多实例部署时各自计数。
//
// 内存上界由 maxKeys 控制：恶意使用海量不同 key 时，map 的规模不会超过
// maxKeys。上界被打满后，新 key 在 AllowAndConsume 里会被**拒绝**（而不是放行）——
// 否则攻击者用随机 key 刷满上界，就能让限流器对后续所有来源失效。
// 被挡掉的次数计入 Evicted，可通过监控观察是否有人在校准防护阈值。
type FailLimiter struct {
	mu      sync.Mutex
	max     int
	window  time.Duration
	maxKeys int
	now     func() time.Time
	items   map[string]*entry
	evicted int64
	lastGC  time.Time
}

type entry struct {
	fails []time.Time
}

// defaultMaxKeys 是 key 数量的默认上界。
//
// 1 万条 × (key 字符串 + 若干 time.Time) 约几 MB 量级，对一个后台服务足够宽裕，
// 同时保证攻击者用随机源 IP 刷不出 OOM。map 的分配失败（OOM）是不可 recover 的。
const defaultMaxKeys = 10000

// New 创建限流器。max <= 0 或 window <= 0 表示关闭，此时所有方法都是空操作。
// maxKeys <= 0 时使用 defaultMaxKeys。
func New(max int, window time.Duration) *FailLimiter {
	return NewWithCap(max, window, defaultMaxKeys)
}

// NewWithCap 与 New 相同，但显式指定 key 数量上界。
func NewWithCap(max int, window time.Duration, maxKeys int) *FailLimiter {
	if maxKeys <= 0 {
		maxKeys = defaultMaxKeys
	}
	return &FailLimiter{
		max:     max,
		window:  window,
		maxKeys: maxKeys,
		now:     time.Now,
		items:   make(map[string]*entry),
	}
}

// Allow 返回该 key 当前是否还能继续尝试。
func (l *FailLimiter) Allow(key string) bool {
	if l == nil || l.max <= 0 || l.window <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.gcLocked(now)
	e, ok := l.items[key]
	if !ok {
		return true
	}
	e.fails = prune(e.fails, now, l.window)
	return len(e.fails) < l.max
}

// AllowAndConsume 原子地完成「检查额度 + 占用一格」。
//
// **为什么必须有这个方法**：Allow 与 Fail 各自加锁，单独调用之间存在窗口。
// 并发 N 个请求可以全部通过 Allow（都读到还剩 1 格），然后全部执行 Fail，
// 于是阈值被突破 N 倍 —— 限流器在最需要它的场景（爆破）下正好失效。
// 调用方若只是想「每请求占一格」，必须用本方法而不是 Allow + Fail 两步。
//
// 返回 true 表示本次请求已占用额度、应当放行。
func (l *FailLimiter) AllowAndConsume(key string) bool {
	if l == nil || l.max <= 0 || l.window <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.gcLocked(now)

	e, ok := l.items[key]
	if !ok {
		// 新 key：先看有没有空位，有则建立条目
		if len(l.items) >= l.maxKeys {
			l.evicted++
			// 上界已满：拒绝而不是放行。之前这里是 return true（放行），
			// 于是攻击者用 10000 个随机 key 打满上界后，所有新 key 都无限放行 ——
			// 限流器在最需要的场景（海量来源爆破）下被完整绕过。宁可误拒，
			// 也不能给爆破留一条「打满即可关掉限流」的路。
			return false
		}
		e = &entry{}
		l.items[key] = e
	}
	e.fails = prune(e.fails, now, l.window)
	if len(e.fails) >= l.max {
		return false
	}
	e.fails = append(e.fails, now)
	return true
}

// Fail 记录一次失败。
func (l *FailLimiter) Fail(key string) {
	if l == nil || l.max <= 0 || l.window <= 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.gcLocked(now)
	e, ok := l.items[key]
	if !ok {
		if len(l.items) >= l.maxKeys {
			l.evicted++
			return // 达到 key 上界，不再为新 key 建条目
		}
		e = &entry{}
		l.items[key] = e
	}
	e.fails = append(prune(e.fails, now, l.window), now)
}

// Reset 清空该 key 的失败记录（成功时调用）。
func (l *FailLimiter) Reset(key string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.items, key)
}

// Stats 返回当前 key 数与被上界挡掉的失败次数，用于监控。
func (l *FailLimiter) Stats() (keys int, evicted int64) {
	if l == nil {
		return 0, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.items), l.evicted
}

// gcLocked 清理已过期条目。调用方必须已持有锁。
func (l *FailLimiter) gcLocked(now time.Time) {
	if now.Sub(l.lastGC) < l.window {
		return
	}
	l.lastGC = now
	for k, e := range l.items {
		e.fails = prune(e.fails, now, l.window)
		if len(e.fails) == 0 {
			delete(l.items, k)
		}
	}
}

// prune 丢弃时间窗之外的记录，返回仍在窗口内的切片。
func prune(fails []time.Time, now time.Time, window time.Duration) []time.Time {
	cut := now.Add(-window)
	i := 0
	for i < len(fails) && !fails[i].After(cut) {
		i++
	}
	if i > 0 {
		fails = append(fails[:0], fails[i:]...)
	}
	return fails
}
