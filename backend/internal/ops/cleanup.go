package ops

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// CleanupOptions 是清理参数。零值字段由配置默认值补齐。
type CleanupOptions struct {
	OpLogDays       int
	LoginLogDays    int
	IdempotencyDays int
	// LoginLockSeconds 是账号锁定窗口：窗口内的登录日志一律不删。
	// 登录失败日志同时是 auth.isLocked 的计数来源，删掉等于「解除所有账号锁定」，
	// 攻击者只要触发锁定再请求一次清理就能恢复无限次尝试。
	LoginLockSeconds int
	BatchSize        int
	// Apply 为 false 时只统计不删除（dry-run）。默认就是 dry-run ——
	// 清理是不可逆的，让「先看一眼会删多少」成为默认动作。
	Apply bool
	Now   time.Time
}

// CleanupStep 是一类数据的处理结果。
type CleanupStep struct {
	Name    string
	Table   string
	Cutoff  time.Time
	Rows    int64
	Batches int
	Note    string
}

// CleanupReport 是整次清理的结果。
type CleanupReport struct {
	DryRun   bool
	Steps    []CleanupStep
	Vacuumed []string
}

// Cleanup 按保留期删除日志与过期幂等键。
//
// 只删这三类，**不动业务数据**：
//
//   - 单据/明细/出入库是账，删了就对不上账，只做归档（导出后另行处理）；
//   - 软删除的主数据（商品、往来单位）不自动物理删除：它们被历史单据引用着，
//     物理删掉后「这笔单卖的是什么」就永久无解。要清理由人看过 check 报告再决定。
//
// 删除分批进行（默认 5000 行一批）：一条 DELETE 删几十万行会长时间持锁、
// 把 WAL 打满，还会让表膨胀得更厉害。分批之后每批都很短，期间业务不受影响。
func Cleanup(db *gorm.DB, opt CleanupOptions) (*CleanupReport, error) {
	if opt.Now.IsZero() {
		opt.Now = time.Now()
	}
	if opt.BatchSize <= 0 {
		opt.BatchSize = 5000
	}
	opCut, loginCut, idemCut := resolveCutoffs(opt)
	rep := &CleanupReport{DryRun: !opt.Apply}

	steps := []struct {
		name  string
		table string
		pk    string
		where string
		args  []any
		cut   time.Time
		note  string
	}{
		{
			name: "操作日志", table: "sys_operation_logs", pk: "id",
			where: "created_at < ?", args: []any{opCut}, cut: opCut,
			note: fmt.Sprintf("保留最近 %d 天", opt.OpLogDays),
		},
		{
			name: "登录日志", table: "sys_login_logs", pk: "id",
			where: "created_at < ?", args: []any{loginCut}, cut: loginCut,
			note: "已按账号锁定窗口兜底，锁定期的记录不会被删",
		},
		{
			name: "过期幂等键", table: "sys_idempotencies", pk: "key",
			where: "expires_at < ?", args: []any{idemCut}, cut: idemCut,
			note: fmt.Sprintf("过期后再保留 %d 天", opt.IdempotencyDays),
		},
	}

	for _, s := range steps {
		rows, batches, err := deleteBatched(db, s.table, s.pk, s.where, s.args, opt.BatchSize, opt.Apply)
		if err != nil {
			return rep, err
		}
		rep.Steps = append(rep.Steps, CleanupStep{
			Name: s.name, Table: s.table, Cutoff: s.cut,
			Rows: rows, Batches: batches, Note: s.note,
		})
	}

	if !opt.Apply {
		return rep, nil
	}

	// 删完立刻 ANALYZE：日志表的查询几乎都带时间范围，统计信息陈旧会让
	// 「最近 7 天」这种查询走错计划。VACUUM（不带 FULL）不会长时间锁表。
	for _, s := range rep.Steps {
		if s.Rows == 0 {
			continue
		}
		if err := db.Exec("VACUUM (ANALYZE) " + s.Table).Error; err != nil {
			return rep, fmt.Errorf("清理 %s 后 VACUUM 失败: %w", s.Table, err)
		}
		rep.Vacuumed = append(rep.Vacuumed, s.Table)
	}
	return rep, nil
}

// resolveCutoffs 计算三类数据的删除边界。
//
// 抽成纯函数是为了能测：边界算错一天，要么少留一天日志，要么白留一年。
func resolveCutoffs(opt CleanupOptions) (opCut, loginCut, idemCut time.Time) {
	now := opt.Now
	if now.IsZero() {
		now = time.Now()
	}
	opCut = now.AddDate(0, 0, -opt.OpLogDays)

	// 登录日志：默认按保留期删，但**锁定窗口内的记录一条都不能动**。
	//
	// 边界要取两者中**更早**的那个：cutoff 语义是「删掉早于它的记录」，
	// 若取更晚的那个（now-5分钟），等于把 5 分钟之前的所有登录日志全删了 ——
	// 既毁了审计，也顺手解除了所有账号锁定（isLocked 靠这批日志计数）。
	loginCut = now.AddDate(0, 0, -opt.LoginLogDays)
	if lock := now.Add(-time.Duration(opt.LoginLockSeconds) * time.Second); lock.Before(loginCut) {
		loginCut = lock
	}

	// 幂等键按 expires_at 判定：过期之后再多留几天，覆盖「过期前提交、
	// 过期后重试」的调用方，让它仍能拿回上一次的结果。
	idemCut = now.AddDate(0, 0, -opt.IdempotencyDays)
	return opCut, loginCut, idemCut
}

// deleteBatched 分批删除；apply=false 时只统计。
//
// 用「先取一批主键再按主键删」而不是一条 DELETE ... LIMIT：
// PostgreSQL 没有 DELETE LIMIT，用子查询取主键是唯一可移植且能限量的写法。
func deleteBatched(db *gorm.DB, table, pk, where string, args []any, batch int, apply bool) (int64, int, error) {
	if !apply {
		var n int64
		if err := db.Raw(fmt.Sprintf("SELECT count(*) FROM %s WHERE %s", table, where), args...).Scan(&n).Error; err != nil {
			return 0, 0, fmt.Errorf("统计 %s 失败: %w", table, err)
		}
		return n, 0, nil
	}

	var total int64
	batches := 0
	for {
		sql := fmt.Sprintf(
			"DELETE FROM %s WHERE %s IN (SELECT %s FROM %s WHERE %s LIMIT %d)",
			table, pk, pk, table, where, batch)
		res := db.Exec(sql, args...)
		if res.Error != nil {
			return total, batches, fmt.Errorf("清理 %s 失败（已删 %d 行）: %w", table, total, res.Error)
		}
		total += res.RowsAffected
		if res.RowsAffected > 0 {
			batches++
		}
		if res.RowsAffected < int64(batch) {
			break
		}
	}
	return total, batches, nil
}
