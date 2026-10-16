package model

import "time"

// 幂等键的记录状态。
const (
	IdempotencyProcessing int16 = 1 // 处理中
	IdempotencyCompleted  int16 = 2 // 已完成
)

// Idempotency 是「幂等键」登记表。
//
// 为什么需要它：导入、批量写入这类操作在客户端请求超时或用户重复点击时会发出
// 两次内容完全相同的请求，服务端如果不识别，就会产生重复数据。调用方在
// Idempotency-Key 头里带一个唯一值，服务端据此登记这次操作的进展与结果：
//   - 首次到达：抢到键，正常执行业务；
//   - 执行中重复到达：返回 409，避免两次并发写；
//   - 已完成后再到达：直接回放上次的响应，调用方拿到与第一次一致的结果。
//
// 不使用 BaseModel：键本身就是主键，这里不需要自增 ID 与软删除 —— 软删除会让
// 「同一个键是否已存在」的判定变复杂，而本表的键恰恰需要能被过期复用。
//
// 过期记录不做主动清理：被复用的键会原地覆盖，因此行数上界是
// 「历史上出现过的不同键数量」，而不是随运行时间无限增长。
type Idempotency struct {
	// Key 即调用方提供的 Idempotency-Key，直接做主键，长度上限与中间件校验一致。
	// （key 在 PostgreSQL 中是非保留关键字，可以直接作为列名。）
	Key string `gorm:"primaryKey;size:128" json:"key"`
	// Scope 是「方法 + 路由模板」，用于防止同一个键被挪到别的接口上复用
	// 而回放到不属于该接口的响应。
	Scope string `gorm:"size:128;not null" json:"scope"`
	// Status 取值见 IdempotencyProcessing / IdempotencyCompleted。
	Status int16 `gorm:"not null" json:"status"`
	// StatusCode 是完成时记录的 HTTP 状态码，回放时原样返回。
	StatusCode int `gorm:"not null" json:"statusCode"`
	// Response 是完成时缓存的响应体，回放时原样返回。
	Response []byte `gorm:"type:bytea" json:"-"`
	// ExpiresAt 到期后该键可被后续请求重新占用。
	// 处理中的行用较短租期，避免进程崩溃后键被无限期占死；
	// 已完成的行用较长保留期，让调用方重试时仍能回放到结果。
	ExpiresAt time.Time `gorm:"index;not null" json:"expiresAt"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updatedAt"`
}

// TableName 指定表名。
func (Idempotency) TableName() string { return "sys_idempotencies" }
