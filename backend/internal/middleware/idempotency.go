package middleware

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm/clause"

	"nova-admin/internal/database"
	"nova-admin/internal/logger"
	"nova-admin/internal/model"
	"nova-admin/internal/pkg/errcode"
	"nova-admin/internal/pkg/response"
)

// IdempotencyKeyHeader 是调用方携带幂等键的请求头名，与主流 API（Stripe 等）一致。
const IdempotencyKeyHeader = "Idempotency-Key"

const (
	// maxIdempotencyKeyLen 与 model.Idempotency.Key 的列宽保持一致。
	maxIdempotencyKeyLen = 128
	// processingLease 是「处理中」记录的租期。业务没跑完时，携带同一键的其它请求
	// 会撞上这条记录并收到 409；超过租期仍未完成，说明持有者已异常退出，
	// 允许后来的请求抢占，避免这个键被永久占死。
	processingLease = 2 * time.Minute
	// completedTTL 是「已完成」结果的保留时长。超过后该键可再次使用，
	// 避免调用方复用旧键时拿到早已过期的回放。
	completedTTL = 24 * time.Hour
)

// Idempotency 提供基于 Idempotency-Key 头的写请求幂等保护。
//
// 未携带该请求头的请求原样放行 —— 幂等是可选的增强能力，不该改变未适配调用方的行为。
// 携带时按下面的状态机处理：
//
//	抢到键            执行处理链；成功后落库缓存响应，失败则释放键（允许修好后重试）
//	已存在且「处理中」  409，拒绝并发重复提交
//	已存在且「已完成」  直接回放上次的响应状态码与响应体
//
// 只应挂在**写接口**上：读接口天然幂等，挂上去只会白白写库。
func Idempotency() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := strings.TrimSpace(c.GetHeader(IdempotencyKeyHeader))
		if key == "" {
			c.Next()
			return
		}
		if len(key) > maxIdempotencyKeyLen {
			response.FailWith(c, errcode.ErrBadRequest,
				fmt.Sprintf("Idempotency-Key 长度不能超过 %d 个字符", maxIdempotencyKeyLen))
			return
		}

		// 路由模板在这里已经可用：本中间件挂在具体路由上，gin 在进入处理链之前
		// 就已完成路由匹配并写好了 fullPath。
		scope := c.Request.Method + " " + c.FullPath()

		record, owned, err := claim(key, scope)
		if err != nil {
			logger.Error("幂等键登记失败",
				zap.String("key", key), zap.String("scope", scope), zap.Error(err))
			response.Fail(c, errcode.Wrap(errcode.ErrDatabase, err))
			return
		}

		if !owned {
			if record.Scope != scope {
				// 同一个键被挪到了别的接口：属于调用方误用，拒绝而不是回放别人的响应
				response.FailWith(c, errcode.ErrConflict,
					"该 Idempotency-Key 已被其它接口使用，请更换")
				return
			}
			if record.Status == model.IdempotencyCompleted {
				replay(c, record)
				return
			}
			response.FailWith(c, errcode.ErrConflict, "相同请求正在处理中，请勿重复提交")
			return
		}

		// 抢到键：接管响应写入以便缓存响应体。必须在 c.Next() 之前替换，
		// 之后处理器里的 c.JSON / c.Data 等都会经由这里写出。
		capture := &bodyCapture{ResponseWriter: c.Writer, body: &bytes.Buffer{}}
		c.Writer = capture
		c.Next()

		status := capture.Status()
		if status >= http.StatusOK && status < http.StatusMultipleChoices {
			complete(key, status, capture.body.Bytes())
			return
		}
		// 失败（含业务校验不通过的 4xx）不缓存：应当允许调用方用同一个键重试
		release(key)
	}
}

// claim 尝试占用 key，返回是否抢到（own）以及未抢到时已存在的那条记录。
//
// 「抢占」用一条带条件更新的 UPSERT 完成：发生冲突时，只有当已存在记录的
// expires_at 已过期才允许覆盖。这样「首次插入」与「抢占过期记录」就是同一个
// 原子语句，并发下只可能有一个请求拿到 RowsAffected == 1，无需额外加锁。
func claim(key, scope string) (model.Idempotency, bool, error) {
	db := database.DB()
	if db == nil {
		return model.Idempotency{}, false, errors.New("数据库未初始化")
	}

	now := time.Now()
	row := model.Idempotency{
		Key:    key,
		Scope:  scope,
		Status: model.IdempotencyProcessing,
		// 抢占后写入的是「处理中」租期；业务成功后会改成 completedTTL。
		ExpiresAt: now.Add(processingLease),
	}
	result := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "key"}},
		// 覆盖列写全：抢占过期记录时，上一条记录的状态码与响应体必须一并清掉，
		// 否则一旦新请求失败被释放、旧响应又被回放，会返回属于上次操作的结果。
		DoUpdates: clause.AssignmentColumns([]string{
			"scope", "status", "status_code", "response", "expires_at", "created_at", "updated_at",
		}),
		Where: clause.Where{Exprs: []clause.Expression{
			clause.Lt{
				Column: clause.Column{Table: model.Idempotency{}.TableName(), Name: "expires_at"},
				Value:  now,
			},
		}},
	}).Create(&row)
	if result.Error != nil {
		return model.Idempotency{}, false, result.Error
	}
	if result.RowsAffected == 1 {
		return row, true, nil
	}

	var existing model.Idempotency
	if err := db.Where("key = ?", key).Take(&existing).Error; err != nil {
		return model.Idempotency{}, false, err
	}
	return existing, false, nil
}

// complete 把键标记为已完成并缓存响应，供后续重复请求回放。
// 失败只记日志：业务本身已经成功返回，不能因为缓存写失败再把请求判成错误。
func complete(key string, status int, body []byte) {
	err := database.DB().Model(&model.Idempotency{}).
		Where("key = ?", key).
		Updates(map[string]any{
			"status":      model.IdempotencyCompleted,
			"status_code": status,
			"response":    body,
			"expires_at":  time.Now().Add(completedTTL),
		}).Error
	if err != nil {
		logger.Warn("幂等响应缓存失败，本次请求的重试将无法回放结果",
			zap.String("key", key), zap.Error(err))
	}
}

// release 释放键，使调用方能够用同一个键重试。
func release(key string) {
	err := database.DB().Delete(&model.Idempotency{}, "key = ?", key).Error
	if err != nil {
		logger.Warn("幂等键释放失败，该键在租期内将无法重试",
			zap.String("key", key), zap.Error(err))
	}
}

// replay 原样回放上次的响应。
func replay(c *gin.Context, record model.Idempotency) {
	c.Header("Content-Type", "application/json; charset=utf-8")
	// 显式标记这是回放结果：排查问题时能立刻区分「真的又执行了一次」与「回放了上次结果」
	c.Header("Idempotent-Replay", "true")
	c.Status(record.StatusCode)
	if len(record.Response) > 0 {
		_, _ = c.Writer.Write(record.Response)
	}
	c.Abort()
}

// bodyCapture 在透传写入的同时把响应体缓存一份，用于幂等回放。
type bodyCapture struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *bodyCapture) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *bodyCapture) WriteString(s string) (int, error) {
	w.body.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}
