package model

import "time"

// OperationLog 操作日志。
type OperationLog struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    int64     `gorm:"index" json:"userId"`
	Username  string    `gorm:"size:64" json:"username"`
	Module    string    `gorm:"size:64;index" json:"module"` // 业务模块
	Action    string    `gorm:"size:64" json:"action"`       // 操作描述
	Method    string    `gorm:"size:16" json:"method"`       // HTTP 方法
	Path      string    `gorm:"size:255" json:"path"`        // 请求路径
	IP        string    `gorm:"size:64" json:"ip"`
	UserAgent string    `gorm:"size:512" json:"userAgent"`
	Request   string    `gorm:"type:text" json:"request"` // 脱敏后的请求体
	Status    int       `gorm:"index" json:"status"`      // HTTP 状态码
	Latency   int64     `gorm:"column:latency_ms" json:"latencyMs"`
	ErrorMsg  string    `gorm:"size:512" json:"errorMsg"`
	CreatedAt time.Time `gorm:"autoCreateTime;index" json:"createdAt"`
}

// TableName 指定表名。
func (OperationLog) TableName() string { return "sys_operation_logs" }

// LoginLog 登录日志。
type LoginLog struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    int64     `gorm:"index" json:"userId"`
	Username  string    `gorm:"size:64;index" json:"username"`
	IP        string    `gorm:"size:64" json:"ip"`
	UserAgent string    `gorm:"size:512" json:"userAgent"`
	Success   bool      `json:"success"`
	Message   string    `gorm:"size:255" json:"message"`
	CreatedAt time.Time `gorm:"autoCreateTime;index" json:"createdAt"`
}

// TableName 指定表名。
func (LoginLog) TableName() string { return "sys_login_logs" }
