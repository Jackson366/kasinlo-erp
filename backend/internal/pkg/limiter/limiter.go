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
// 限制：状态在进程内存里，多实例部署时各自计数；GC 依赖窗口期，
// 恶意使用海量不同 key 时内存增长上界 ≈ 攻击速率 × 窗口时长。
type FailLimiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	now    func() time.Time
	items  map[string]*entry
	lastGC time.Time
}

type entry struct {
	fails []time.Time
}

// New 创建限流器。max <= 0 或 window <= 0 表示关闭，此时所有方法都是空操作。
func New(max int, window time.Duration) *FailLimiter {
	return &FailLimiter{
		max:    max,
		window: window,
		now:    time.Now,
		items:  make(map[string]*entry),
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
