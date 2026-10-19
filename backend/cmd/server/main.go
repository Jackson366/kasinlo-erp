package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"

	"nova-erp/internal/assembly"
	"nova-erp/internal/bootstrap"
	"nova-erp/internal/database"
	"nova-erp/internal/logger"
	"nova-erp/internal/oplog"
	"nova-erp/internal/router"
)

// Nova ERP 服务端入口。
//
// @title                      Nova ERP API
// @version                    1.0
// @description                基于 Gin + GORM + PostgreSQL 的权限管理后台后端。
// @description                所有接口统一返回 {code, message, data, traceId}，业务码 0 表示成功。
// @description                业务码前三位即 HTTP 状态码：40000→400、40100~40106→401、40300→403、40400→404、40900→409、42900→429、50000/50100→500。
// @description                除「登录」「刷新令牌」外，所有接口都需要请求头 `Authorization: Bearer <accessToken>`。
// @description                接口前缀由 config.yaml 的 app.prefix + app.version 决定，服务启动时会同步到本文档。
// @termsOfService             https://example.com/terms/
// @contact.name               Nova ERP 开发组
// @license.name               MIT
// @BasePath                   /api/v1
// @schemes                    http https
// @securityDefinitions.apikey BearerAuth
// @in                         header
// @name                       Authorization
// @description                填写 "Bearer <accessToken>"，注意 Bearer 与令牌之间有一个空格
func main() {
	configPath := flag.String("config", "", "配置文件路径（默认 <程序所在目录>/config.yaml）")
	seedOnly := flag.Bool("seed", false, "仅执行数据库迁移与种子数据初始化后退出")
	flag.Parse()

	// 默认配置文件钉在 exe 同级目录，而不是进程工作目录：
	// 以 Windows 服务 / 计划任务启动时 CWD 是 C:\Windows\System32，
	// 按 CWD 找 config.yaml 会直接「配置文件不存在」，而现场往往
	// 只看到一句启动失败，不知道是工作目录的问题。
	path := strings.TrimSpace(*configPath)
	if path == "" {
		path = defaultConfigPath()
	}

	if err := run(path, *seedOnly); err != nil {
		fmt.Fprintf(os.Stderr, "服务启动失败: %v\n", err)
		os.Exit(1)
	}
}

// defaultConfigPath 返回 <可执行文件所在目录>/config.yaml。
func defaultConfigPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "config.yaml"
	}
	return filepath.Join(filepath.Dir(exe), "config.yaml")
}

func run(configPath string, seedOnly bool) error {
	// 1. 基础设施初始化（配置 / 日志 / 令牌 / 数据库 / 迁移 / 模块装配 / 种子数据）
	//    模块清单来自组装层：这里是全项目唯一知道「有哪些模块」的调用点。
	res, err := bootstrap.Init(configPath, bootstrap.Options{
		SeedData: true,
		Modules:  assembly.Modules,
	})
	if err != nil {
		return err
	}
	cfg := res.Config
	defer logger.Sync()
	defer func() { _ = database.Close() }()

	if seedOnly {
		logger.Info("数据库初始化完成，已退出")
		return nil
	}

	// 2. 启动异步操作日志
	oplog.Init(res.DB)
	defer oplog.Shutdown(context.Background())

	// 3. 启动 HTTP 服务
	srv := &http.Server{
		Addr:              cfg.App.Addr(),
		Handler:           router.New(cfg),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.L().Info("HTTP 服务已启动",
			zap.String("addr", cfg.App.Addr()),
			zap.String("basePath", cfg.App.BasePath()),
			zap.String("mode", cfg.App.Mode),
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	// 4. 等待退出信号并优雅关停
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return fmt.Errorf("HTTP 服务异常退出: %w", err)
	case sig := <-quit:
		logger.Info("收到退出信号，开始优雅关停", zap.String("signal", sig.String()))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("优雅关停失败: %w", err)
	}
	logger.Info("服务已安全退出")
	return nil
}
