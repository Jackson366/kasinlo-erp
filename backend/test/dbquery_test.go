package test

import (
	"errors"
	"testing"

	"nova-admin/internal/database"
	"nova-admin/internal/model"
	"nova-admin/internal/pkg/dbquery"
	"nova-admin/internal/pkg/errcode"
)

// TestFetchWithCap 验证「带上限取数」的核心约定：
// 内部多查一行用于判断是否超限，返回行数永不越过上限。
// 这条 +1 一旦被漏掉，超限判断会静默失效、全表被读进内存，而没有任何测试会红。
func TestFetchWithCap(t *testing.T) {
	setup(t)
	db := database.DB()

	var total int64
	if err := db.Model(&model.Menu{}).Count(&total).Error; err != nil {
		t.Fatalf("统计菜单数量失败: %v", err)
	}
	if total < 3 {
		t.Fatalf("种子菜单数量过少（%d 条），无法验证上限行为", total)
	}

	// ---- 上限小于总行数：返回 cap 行并报告还有更多 ----
	rows, exceeded, err := dbquery.FetchWithCap[model.Menu](db, 2)
	if err != nil {
		t.Fatalf("取数失败: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("应返回 2 行，实际 %d 行", len(rows))
	}
	if !exceeded {
		t.Fatalf("实际还有更多行，exceeded 应为 true")
	}

	// ---- 上限等于总行数：返回全部且不应报超限 ----
	rows, exceeded, err = dbquery.FetchWithCap[model.Menu](db, int(total))
	if err != nil {
		t.Fatalf("取数失败: %v", err)
	}
	if len(rows) != int(total) {
		t.Fatalf("应返回全部 %d 行，实际 %d 行", total, len(rows))
	}
	if exceeded {
		t.Fatalf("恰好等于上限时不应报超限")
	}

	// ---- 上限大于总行数：同样返回全部且不报超限 ----
	rows, exceeded, err = dbquery.FetchWithCap[model.Menu](db, int(total)+100)
	if err != nil {
		t.Fatalf("取数失败: %v", err)
	}
	if len(rows) != int(total) || exceeded {
		t.Fatalf("上限大于总行数时应返回全部 %d 行且不报超限，实际 %d 行 exceeded=%v",
			total, len(rows), exceeded)
	}
}

// TestFetchWithCapWrapsDatabaseError 确认数据库错误被归一为业务错误码，
// 调用方不需要再包一层，也不会把驱动原始错误直接暴露给前端。
func TestFetchWithCapWrapsDatabaseError(t *testing.T) {
	setup(t)

	_, _, err := dbquery.FetchWithCap[model.Menu](database.DB().Table("table_that_does_not_exist"), 1)
	if err == nil {
		t.Fatalf("查询不存在的表应返回错误")
	}
	var be *errcode.Error
	if !errors.As(err, &be) {
		t.Fatalf("错误类型应为 *errcode.Error，实际 %T", err)
	}
	if be.Code != errcode.ErrDatabase {
		t.Fatalf("错误码应为 ErrDatabase，实际 %d", be.Code)
	}
	if !errors.Is(err, be.Err) {
		t.Fatalf("应保留底层错误以便排查")
	}
}
