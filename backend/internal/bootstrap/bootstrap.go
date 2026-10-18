// Package bootstrap 负责服务启动时的基础设施初始化。
// 抽成独立包后，main 与端到端测试可以复用同一套初始化流程，避免行为漂移。
package bootstrap

import (
	"fmt"

	"gorm.io/gorm"

	"nova-erp/internal/config"
	"nova-erp/internal/database"
	"nova-erp/internal/logger"
	"nova-erp/internal/module"
	"nova-erp/internal/ops"
	"nova-erp/internal/pkg/jwt"
	"nova-erp/internal/seed"
)

// Result 是初始化结果。
type Result struct {
	Config *config.Config
	DB     *gorm.DB
}

// Options 是启动选项。
type Options struct {
	// SeedData 为 false 时跳过种子数据初始化（用于不需要演示数据的场景）。
	SeedData bool

	// Modules 返回本进程要装配的全部模块，由组装层（internal/assembly）提供。
	//
	// 之所以传函数而不是直接 import：本包属于框架层，一旦 import 了具体模块，
	// 业务模块就再也删不掉了（编译期就依赖它）。传函数之后，「谁知道有哪些模块」
	// 这件事被完整地留在组装层，框架层保持零业务依赖。
	Modules func(*config.Config) []module.Module
}

// Init 按顺序完成：加载配置 -> 初始化日志 -> 初始化令牌 -> 连接数据库 ->
// 迁移表结构 -> 装配模块 -> 写入种子数据。
func Init(configPath string, opts Options) (*Result, error) {
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
	// 迁移前先判断方向：库比程序新就拒绝启动（回滚时最容易踩）。
	// 必须在 AutoMigrate 之前 —— 它一旦动手就可能按旧代码误改新结构。
	if _, err := database.CheckSchemaVersion(db); err != nil {
		return nil, err
	}
	if err := database.AutoMigrate(); err != nil {
		return nil, fmt.Errorf("自动迁移表结构失败: %w", err)
	}
	if err := database.EnsureIndexes(); err != nil {
		return nil, fmt.Errorf("创建索引失败: %w", err)
	}
	// 结构就位后再修补历史数据：修补语句依赖列已经存在（老库升级时
	// 列可能是本次 AutoMigrate 才加上或改可空的）。
	if err := database.PatchData(); err != nil {
		return nil, err
	}
	// 结构就位、数据修补完成之后才把版本推进：中途失败时版本仍停在旧值，
	// 下次启动会重新走一遍（AutoMigrate 与 PatchData 都是幂等的）。
	if err := database.RecordSchemaVersion(database.DB()); err != nil {
		return nil, err
	}
	// 核对上一次升级是否真的生效：升级执行只是把新程序落位成 .new，
	// 生效与否只有现在这个进程知道。放在种子数据之前 ——
	// 早一步发现「版本没换过来」，比带着错版本的种子数据跑一整天再发现要好。
	if err := ops.FinalizeUpgrades(database.DB(), cfg.App.Release); err != nil {
		return nil, err
	}

	// 装配必须早于种子数据：内置菜单是从模块清单聚合出来的，
	// 登记表还空着就播种，只会写出一张空菜单表 —— 而服务照样能起来。
	if opts.Modules != nil {
		if err := module.Build(opts.Modules(cfg)...); err != nil {
			return nil, fmt.Errorf("模块装配失败: %w", err)
		}
	}
	if opts.SeedData {
		if err := seed.Run(); err != nil {
			return nil, err
		}
	}
	return &Result{Config: cfg, DB: db}, nil
}
