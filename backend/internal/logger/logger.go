package logger

import (
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"nova-admin/internal/config"
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

	// 同时落盘，便于排查线上问题
	if cfg.Dir != "" {
		if err := os.MkdirAll(cfg.Dir, 0o755); err != nil {
			return err
		}
		file := cfg.File
		if file == "" {
			file = "app.log"
		}
		f, err := os.OpenFile(filepath.Join(cfg.Dir, file), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		fileEnc := zapcore.NewConsoleEncoder(encCfg)
		cores = append(cores, zapcore.NewCore(fileEnc, zapcore.AddSync(f), level))
	}

	opts := []zap.Option{zap.AddCaller()}
	if debug {
		opts = append(opts, zap.AddStacktrace(zapcore.ErrorLevel))
	}
	log = zap.New(zapcore.NewTee(cores...), opts...)
	return nil
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
func Debug(msg string, fields ...zap.Field) { L().Debug(msg, fields...) }
func Info(msg string, fields ...zap.Field)  { L().Info(msg, fields...) }
func Warn(msg string, fields ...zap.Field)  { L().Warn(msg, fields...) }
func Error(msg string, fields ...zap.Field) { L().Error(msg, fields...) }
func Fatal(msg string, fields ...zap.Field) { L().Fatal(msg, fields...) }
