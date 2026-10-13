package inv

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"nova-erp/internal/database"
	"nova-erp/internal/model"
	"nova-erp/internal/pkg/dberr"
	"nova-erp/internal/pkg/errcode"
)

// Service 进销存服务。所有业务逻辑收敛在这里，不感知 HTTP。
type Service struct {
	db *gorm.DB
}

// NewService 创建进销存服务。
func NewService() *Service { return &Service{db: database.DB()} }

// saveErr 保留写入路径上抛出的业务错误码，其余归一到数据库错误。
//
// 不能无脑 Wrap(ErrDatabase)：事务里「库存不足」「已审核单据不可删除」这类
// 校验是带错误码抛出来的，套一层就把 40900 冲成 50100「数据库操作失败」，
// 用户只看到一句误导性的提示，也不知道该改什么（user 模块的 saveErr 同一个坑）。
func saveErr(err error) error {
	if err == nil {
		return nil // 允许直接包在 return saveErr(txErr) 这种写法里
	}
	var be *errcode.Error
	if errors.As(err, &be) {
		return be
	}
	// 并发撞编号 / 单号的部分唯一索引（23505）：「先查后插」存在 TOCTOU 窗口，
	// 数据库兜住数据正确性，错误码要归一成 409 冲突而不是 50100。
	if dberr.IsUniqueViolation(err) {
		return errcode.New(errcode.ErrConflict, "编号已被占用，请重试")
	}
	return errcode.Wrap(errcode.ErrDatabase, err)
}

// round2 金额与数量统一保留两位小数，避免浮点误差在汇总里累积。
func round2(v float64) float64 { return math.Round(v*100) / 100 }

// round4 基本数量按 4 位小数归一。
//
// 换算是双向的：1 米 = 0.1667 根，只保留两位小数的话，「6 米」折算回来
// 只有 1.00 根，误差会随着每次出入库一点点吃掉库存。金额可以只到分，
// 数量不行 —— 它与换算率的分辨率必须一致（numeric(14,4)）。
func round4(v float64) float64 { return math.Round(v*10000) / 10000 }

// fmtQty 把数量格式化成不带多余零的字符串，用于错误提示里报「现有多少」。
func fmtQty(v float64) string { return strconv.FormatFloat(round4(v), 'f', -1, 64) }

// likeKW 把用户输入的关键字包成 ILIKE 的「包含」模式，并转义通配符。
//
// 不转义的后果很隐蔽：PostgreSQL 的 LIKE 里 `%` 匹配任意串、`_` 匹配单个字符，
// 于是现场搜「M12_40」会连「M12X40」一起搜出来，搜「50%」等于搜「50」开头的
// 一切 —— 结果既不报错也不为空，只是**静默地不是你要的那批**。
// 用户无从判断「是没找到，还是被通配符吃掉了」，只会以为系统数据不对。
// 转义符统一用 `\`，所以每条 ILIKE 都要跟着写 ESCAPE '\'。
func likeKW(keyword string) string {
	esc := strings.ReplaceAll(keyword, `\`, `\\`)
	esc = strings.ReplaceAll(esc, `%`, `\%`)
	esc = strings.ReplaceAll(esc, `_`, `\_`)
	return "%" + esc + "%"
}

// likeExpr 生成带转义的 ILIKE 片段，列已由调用方给出。
// 例：likeExpr("p.name") -> `p.name ILIKE ? ESCAPE '\'`
func likeExpr(col string) string { return col + ` ILIKE ? ESCAPE '\'` }

// parseDate 解析 yyyy-MM-dd；空串返回零值时间（表示未填）。
// 同时兼容 RFC3339，前端日期选择器两种都可能给出。
func parseDate(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	for _, layout := range []string{"2006-01-02", time.RFC3339} {
		if t, err := time.ParseInLocation(layout, s, database.Location()); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errcode.Newf(errcode.ErrBadRequest, "日期格式非法：%s（应为 yyyy-MM-dd）", s)
}

// parseDatePtr 解析「选填」日期：空串返回 nil，表示数据库里存 NULL。
//
// 只给可选字段（目前是单据到期日）用。必填字段（单据日期等）仍走 parseDate：
// 让「空值」和「零值时间」在类型上就分开，避免 0001-01-01 这种
// 「看起来有值、实际没有」的数据再被写进库。
func parseDatePtr(s string) (*time.Time, error) {
	t, err := parseDate(s)
	if err != nil {
		return nil, err
	}
	if t.IsZero() {
		return nil, nil
	}
	return &t, nil
}

// nextNo 生成业务单号：前缀 + 当天日期 + 4 位流水。
// 唯一性最终由部分唯一索引兜底，这里的流水只负责「看起来连续」。
func (s *Service) nextNo(table, prefix string) (string, error) {
	day := time.Now().Format("20060102")
	var count int64
	if err := s.db.Table(table).Where("order_no LIKE ?", prefix+day+"%").Count(&count).Error; err != nil {
		return "", errcode.Wrap(errcode.ErrDatabase, err)
	}
	return fmt.Sprintf("%s%s%04d", prefix, day, count+1), nil
}

// settleStatus 由「总额 - 已结算」推导结算状态。
func settleStatus(total, settled float64) string {
	if settled <= 0 {
		return model.InvSettleUnpaid
	}
	if settled+0.001 >= total {
		return model.InvSettlePaid
	}
	return model.InvSettlePartial
}

// overview 用的时间范围：本月起点。
func monthStart() time.Time {
	now := time.Now().In(database.Location())
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, database.Location())
}

// rangeStart 把报表时间范围映射为起点时间。无法识别时回退到本月。
func rangeStart(rng string) (time.Time, string) {
	now := time.Now().In(database.Location())
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, database.Location())
	switch rng {
	case "today":
		return today, "今日"
	case "week":
		// 周一为一周起点
		offset := (int(now.Weekday()) + 6) % 7
		return today.AddDate(0, 0, -offset), "本周"
	case "quarter":
		q := (int(now.Month()) - 1) / 3
		return time.Date(now.Year(), time.Month(q*3+1), 1, 0, 0, 0, 0, database.Location()), "本季"
	case "year":
		return time.Date(now.Year(), 1, 1, 0, 0, 0, 0, database.Location()), "本年"
	default:
		return monthStart(), "本月"
	}
}
