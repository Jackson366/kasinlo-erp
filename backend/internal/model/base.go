package model

import (
	"time"

	"gorm.io/gorm"
)

// BaseModel 是所有业务表的公共字段。
type BaseModel struct {
	ID        int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time      `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// 通用状态值
const (
	StatusDisabled int16 = 0 // 禁用
	StatusEnabled  int16 = 1 // 启用
)
