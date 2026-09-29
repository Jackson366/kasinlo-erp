package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"nova-admin/internal/bootstrap"
	"nova-admin/internal/database"
	"nova-admin/internal/logger"
	"nova-admin/internal/oplog"
	"nova-admin/internal/router"
)

func main() {
	configPath := flag.String("config", "config.yaml", "配置文件路径")
	seedOnly := flag.Bool("seed", false, "仅执行数据库迁移与种子数据初始化后退出")
	flag.Parse()

	if err := run(*configPath, *seedOnly); err != nil {
		fmt.Fprintf(os.Stderr, "服务启动失败: %v\n", err)
		os.Exit(1)
	}
}

func run(configPath string, seedOnly bool) error {
	// 1. 基础设施初始化（配置 / 日志 / 令牌 / 数据库 / 迁移 / 种子数据）
	res, err := bootstrap.Init(configPath, true)
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
