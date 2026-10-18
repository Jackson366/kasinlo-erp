package ops

import (
	"sort"
	"time"
)

// BackupFile 是一个已落地的备份文件。
type BackupFile struct {
	Path string    // .dump 的绝对路径
	Time time.Time // 备份时间（取文件名里的时间戳，取不到时回落到 mtime）
	Size int64
}

// PlanRetention 计算「该删哪些备份」。
//
// 策略是 GFS 的简化版，只保留两层：
//   - **最近 keepCount 份**一律保留：覆盖「昨天刚改错了，回滚一下」这类日常场景；
//   - **每个月最新的一份**保留，覆盖最近 keepMonths 个月：覆盖「上个月对账发现
//     少了一笔」这类需要往前翻的场景。
//
// 刻意不做「周档」：三层保留对这套系统（单库、几十 MB、日增长几千行）是过度设计，
// 真正要防的是「备份目录被写满」和「只能回滚一天」，月档已经够。
//
// 两条硬约束：
//  1. **最新一份永不删除** —— 哪怕参数写错（keepCount=0）也不能把唯一备份删掉；
//  2. 时间相同时按路径排序，保证结果稳定（同一批文件反复调用得到同一份清单），
//     否则清理动作会变成「随机删一个」。
func PlanRetention(files []BackupFile, now time.Time, keepCount, keepMonths int) []BackupFile {
	if len(files) == 0 {
		return nil
	}
	if keepCount < 1 {
		keepCount = 1
	}

	sorted := make([]BackupFile, len(files))
	copy(sorted, files)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Time.Equal(sorted[j].Time) {
			return sorted[i].Path > sorted[j].Path
		}
		return sorted[i].Time.After(sorted[j].Time)
	})

	keep := make(map[string]bool, len(sorted))
	for i, f := range sorted {
		if i < keepCount {
			keep[f.Path] = true
		}
	}
	// 月档：每个月里最新的那一份。
	// i 从 0（本月）开始，到 keepMonths-1 为止 —— keepMonths 的语义是
	// 「一共覆盖几个月」，不是「往前再数几个月」。
	seenMonth := make(map[string]bool)
	for i := 0; i < keepMonths && i <= len(sorted); i++ {
		month := now.AddDate(0, -i, 0).Format("2006-01")
		if seenMonth[month] {
			continue
		}
		for _, f := range sorted {
			if f.Time.Format("2006-01") == month {
				keep[f.Path] = true
				seenMonth[month] = true
				break
			}
		}
	}
	// 硬约束：无论如何都留下最新一份
	keep[sorted[0].Path] = true

	out := make([]BackupFile, 0, len(sorted))
	for _, f := range sorted {
		if !keep[f.Path] {
			out = append(out, f)
		}
	}
	return out
}
