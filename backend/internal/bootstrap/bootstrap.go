// Package bootstrap 负责服务启动时的基础设施初始化。
// 抽成独立包后，main 与端到端测试可以复用同一套初始化流程，避免行为漂移。
package bootstrap

import (
	"fmt"

	"gorm.io/gorm"

	"nova-admin/internal/config"
	"nova-admin/internal/database"
	"nova-admin/internal/logger"
	"nova-admin/internal/pkg/jwt"
	"nova-admin/internal/seed"
)

// Result 是初始化结果。
type Result struct {
	Config *config.Config
	DB     *gorm.DB
}

// Init 按顺序完成：加载配置 -> 初始化日志 -> 初始化令牌 -> 连接数据库 -> 迁移表结构 -> 写入种子数据。
// seedData 为 false 时跳过种子数据初始化（用于不需要演示数据的场景）。
func Init(configPath string, seedData bool) (*Result, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	if err := logger.Init(cfg.Log, cfg.App.IsDebug()); err != nil {
		return nil, fmt.Errorf("初始化日志失败: %w", err)
	}
	jwt.Init(cfg.JWT)

	db, err := database.Init(cfg.Database)
	if err != nil {
		return nil, err
	}
	if err := database.AutoMigrate(); err != nil {
		return nil, fmt.Errorf("自动迁移表结构失败: %w", err)
	}
	if err := database.EnsureIndexes(); err != nil {
		return nil, fmt.Errorf("创建索引失败: %w", err)
	}
	if seedData {
		if err := seed.Run(); err != nil {
			return nil, err
		}
	}
	return &Result{Config: cfg, DB: db}, nil
}
