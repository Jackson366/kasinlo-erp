// Package oplog 提供异步操作日志写入能力。
// 采用「缓冲通道 + 单消费者批量落库」模型，避免日志写入拖慢接口响应。
package oplog

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"nova-admin/internal/logger"
	"nova-admin/internal/model"
	"nova-admin/internal/pkg/textutil"
)

// Entry 是一条待写入的操作日志。
type Entry struct {
	UserID    int64
	Username  string
	Module    string
	Action    string
	Method    string
	Path      string
	IP        string
	UserAgent string
	Request   string
	Status    int
	LatencyMS int64
	ErrorMsg  string
	CreatedAt time.Time
}

const (
	defaultBuffer   = 2048
	batchSize       = 64
	flushInterval   = 2 * time.Second
	maxRequestBody  = 2000 // 单条请求体最大留存字符数
	dropLogInterval = 5 * time.Second
)

var (
	ch        chan Entry
	syncCh    chan chan struct{}
	dbRef     *gorm.DB
	once      sync.Once
	closeOnce sync.Once
	lastDrop  time.Time
	dropMu    sync.Mutex
	dropped   int64

	// writeMu 把「关通道」和「往通道发」变成互斥操作：
	// Write 全程持 RLock 做非阻塞发送，Shutdown 持 Lock 置位 closed 再 close(ch)。
	// 没有它，srv.Shutdown 超时后 defer 的 oplog.Shutdown 关闭通道，
	// 仍在收尾的 handler 再调 Write 就是 send on closed channel —— 直接 panic 崩进程。
	writeMu          sync.RWMutex
	closed           bool
	shutdownWarnOnce sync.Once
)

// reportAfterShutdown 在关断后收到写入时打一条 WARN（仅一次）。
// 优雅关停窗口内存活请求写完日志是正常现象，不值得 ERROR；
// 但必须留痕，否则「退出前最后一批日志去哪了」永远说不清。
func reportAfterShutdown() {
	shutdownWarnOnce.Do(func() {
		logger.Warn("操作日志已关断，关断后的写入被丢弃",
			zap.String("场景", "优雅关停窗口内仍有请求在写日志"))
	})
}

// Init 启动日志写入协程。db 为数据库句柄。
func Init(db *gorm.DB) {
	once.Do(func() {
		dbRef = db
		ch = make(chan Entry, defaultBuffer)
		syncCh = make(chan chan struct{})
		go consume()
	})
}

// Write 非阻塞写入一条日志；缓冲满时丢弃并计数。
//
// **不要**在 ch == nil 时静默 return：那意味着 Init 没被调用（main 里忘了调、
// 某个测试直接构造了 Service），此时**所有**审计日志都在无声消失，
// 而页面上看不到任何异常 —— 审计链断了却没人知道。
// 这种情况要明确记一条 ERROR，且只记一次（否则每个请求刷一行日志，反过来把日志淹掉）。
//
// 同理，关断后（closed）也不能静默 return：记一条 WARN 留痕后返回。
// 整个发送过程持 writeMu.RLock，保证不会和 Shutdown 的 close(ch) 交叠。
func Write(e Entry) {
	writeMu.RLock()
	defer writeMu.RUnlock()
	if closed {
		reportAfterShutdown()
		return
	}
	if ch == nil {
		reportUninitialized()
		return
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}
	select {
	case ch <- e:
	default:
		recordDrop()
	}
}

var (
	initWarnOnce sync.Once
)

// reportUninitialized 在未初始化时打一条 ERROR（仅一次）。
func reportUninitialized() {
	initWarnOnce.Do(func() {
		logger.Error("操作日志未初始化，审计日志全部丢失",
			zap.String("原因", "oplog.Init 未被调用"),
			zap.String("后果", "所有操作审计记录不会落库"))
	})
}

// Flush 阻塞直到当前缓冲区中的日志全部落库。
// 优雅关停与测试断言前调用，确保日志已持久化。
func Flush(timeout time.Duration) error {
	if ch == nil || syncCh == nil {
		return nil
	}
	done := make(chan struct{})
	select {
	case syncCh <- done:
	case <-time.After(timeout):
		return errors.New("oplog: 发送刷盘指令超时")
	}
	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return errors.New("oplog: 等待刷盘完成超时")
	}
}

// Shutdown 先把缓冲日志刷盘，再关闭通道。ctx 用于约束刷盘等待上限。
// 关通道前先置 closed，此后迟到的 Write 直接丢弃并留痕，不会 panic。
func Shutdown(ctx context.Context) {
	closeOnce.Do(func() {
		if ch == nil {
			return
		}
		timeout := 2 * time.Second
		if deadline, ok := ctx.Deadline(); ok {
			if d := time.Until(deadline); d > 0 && d < timeout {
				timeout = d
			}
		}
		if err := Flush(timeout); err != nil {
			logger.Warn("退出前刷新操作日志超时", zap.Error(err))
		}
		writeMu.Lock()
		closed = true
		close(ch)
		writeMu.Unlock()
	})
}

func consume() {
	buf := make([]Entry, 0, batchSize)
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	flush := func() {
		if len(buf) == 0 {
			return
		}
		rows := make([]model.OperationLog, 0, len(buf))
		for _, e := range buf {
			rows = append(rows, model.OperationLog{
				UserID:    e.UserID,
				Username:  textutil.Fit(e.Username, colUsername),
				Module:    textutil.Fit(e.Module, colModule),
				Action:    textutil.Fit(e.Action, colAction),
				Method:    textutil.Fit(e.Method, colMethod),
				Path:      textutil.Fit(e.Path, colPath),
				IP:        textutil.Fit(e.IP, colIP),
				UserAgent: textutil.Fit(e.UserAgent, colUserAgent),
				Request:   textutil.Fit(e.Request, maxRequestBody),
				Status:    e.Status,
				Latency:   e.LatencyMS,
				ErrorMsg:  textutil.Fit(e.ErrorMsg, colErrorMsg),
				CreatedAt: e.CreatedAt,
			})
		}
		if err := dbRef.CreateInBatches(rows, batchSize).Error; err != nil {
			logger.Warn("操作日志写入失败", zap.Error(err))
		}
		buf = buf[:0]
	}

	for {
		select {
		case e, ok := <-ch:
			if !ok {
				flush()
				return
			}
			buf = append(buf, e)
			if len(buf) >= batchSize {
				flush()
			}
		case done := <-syncCh:
			// 先把通道里排队的事件取空，再落库
			for {
				select {
				case e, ok := <-ch:
					if !ok {
						flush()
						close(done)
						return
					}
					buf = append(buf, e)
					continue
				default:
				}
				break
			}
			flush()
			close(done)
		case <-ticker.C:
			flush()
		}
	}
}

func recordDrop() {
	dropMu.Lock()
	defer dropMu.Unlock()
	dropped++
	if time.Since(lastDrop) > dropLogInterval {
		logger.Warn("操作日志缓冲已满，丢弃日志", zap.Int64("dropped", dropped))
		lastDrop = time.Now()
	}
}

// 各列的长度上限，必须与 model.OperationLog 的 gorm size 标签严格一致。
//
// 这些字段里有相当一部分直接来自客户端（Path / UserAgent / Request / ErrorMsg）。
// 全部收敛在写入边界上处理，原因有两个，都指向同一个后果「审计记录凭空消失」：
//
//  1. 超长会被 PostgreSQL 以 22001（value too long for type character varying(n)）拒绝。
//     曾经 Path 完全没有截断，一个超长 URL 就足以让这条日志写不进去。
//  2. 非法 UTF-8 会被以 22021 拒绝。上传接口的 multipart 请求体就是这样中招的。
//
// 收敛逻辑统一走 textutil.Fit（按字符截断 + ToValidUTF8），不要在这里另写一份。
const (
	colUsername  = 64
	colModule    = 64
	colAction    = 64
	colMethod    = 16
	colPath      = 255
	colIP        = 64
	colUserAgent = 512
	colErrorMsg  = 512
)
