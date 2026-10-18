package ops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 备份文件名只到秒，同一秒两次备份会写同一个 target 互相截断。
// 占位必须原子且撞名要报错，否则留下的是「看起来存在、恢复才发现坏」的备份。
func TestReserveBackupFileRejectsSameSecondCollision(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "nova-20261018-214300.dump")

	if err := reserveBackupFile(target); err != nil {
		t.Fatalf("首次占位应成功: %v", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("占位后文件应存在: %v", err)
	}
	if info.Size() != 0 {
		t.Fatalf("占位文件应为空，实际 %d 字节", info.Size())
	}

	err = reserveBackupFile(target)
	if err == nil {
		t.Fatal("同一秒第二次占位应报错（否则两个 pg_dump 会互相截断）")
	}
	if !strings.Contains(err.Error(), "已存在") {
		t.Fatalf("报错应说明是同名备份在生成，实际: %v", err)
	}
}

// 目录不存在时应当明确失败，而不是在 pg_dump 阶段才炸出一个更难懂的错误。
func TestReserveBackupFileFailsOnMissingDir(t *testing.T) {
	dir := t.TempDir()
	err := reserveBackupFile(filepath.Join(dir, "nope", "x.dump"))
	if err == nil {
		t.Fatal("目录不存在时占位应失败")
	}
	if strings.Contains(err.Error(), "已存在") {
		t.Fatalf("不该把「目录不存在」报成「备份已存在」: %v", err)
	}
}
