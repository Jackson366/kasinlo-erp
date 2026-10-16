package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"

	"nova-erp/internal/config"
)

var log *zap.Logger

// Init 初始化全局日志器。
func Init(cfg config.LogConfig, debug bool) error {
	level := zapcore.InfoLevel
	if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
		level = zapcore.InfoLevel
	}

	encCfg := zap.NewProductionEncoderConfig()
	encCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	encCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
	encCfg.EncodeCaller = zapcore.ShortCallerEncoder

	var encoder zapcore.Encoder
	if cfg.Format == "json" {
		encoder = zapcore.NewJSONEncoder(encCfg)
	} else {
		encoder = zapcore.NewConsoleEncoder(encCfg)
	}

	cores := []zapcore.Core{
		zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), level),
	}

	// 同时落盘，便于排查线上问题。
	// lumberjack 负责轮转（单文件超限切分、保留 N 个、过期清理、旧档压缩），
	// 否则 O_APPEND 直写的单文件会随运行时间无限增长，最终吃满磁盘。
	if cfg.Dir != "" {
		file := cfg.File
		if file == "" {
			file = "app.log"
		}
		rotate := &lumberjack.Logger{
			Filename:   cfg.Dir + string(os.PathSeparator) + file,
			MaxSize:    orDefault(cfg.MaxSizeMB, 100),
			MaxBackups: orDefault(cfg.MaxBackups, 7),
			MaxAge:     orDefault(cfg.MaxAgeDays, 30),
			Compress:   true,
		}
		fileEnc := zapcore.NewConsoleEncoder(encCfg)
		cores = append(cores, zapcore.NewCore(fileEnc, zapcore.AddSync(rotate), level))
	}

	opts := []zap.Option{zap.AddCaller()}
	if debug {
		opts = append(opts, zap.AddStacktrace(zapcore.ErrorLevel))
	}
	log = zap.New(zapcore.NewTee(cores...), opts...)
	return nil
}

// orDefault 返回 v；v 未配置（<=0）时用 def。
func orDefault(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}

// L 返回全局日志器，未初始化时返回 no-op 实例。
func L() *zap.Logger {
	if log == nil {
		return zap.NewNop()
	}
	return log
}

// Sync 刷新缓冲日志。
func Sync() {
	if log != nil {
		_ = log.Sync()
	}
}

// 便捷方法
func Info(msg string, fields ...zap.Field)  { L().Info(msg, fields...) }
func Warn(msg string, fields ...zap.Field)  { L().Warn(msg, fields...) }
func Error(msg string, fields ...zap.Field) { L().Error(msg, fields...) }
