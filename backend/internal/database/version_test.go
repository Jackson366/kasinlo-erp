package database

import (
	"strings"
	"testing"
)

func TestCompareSchemaVersion(t *testing.T) {
	cases := []struct {
		db, bin int32
		wantErr bool
	}{
		{1, 1, false}, // 版本一致
		{0, 1, false}, // 新库 / 老库未登记：交给 AutoMigrate 补齐
		{1, 2, false}, // 程序比库新：正常升级
		{2, 1, true},  // 库比程序新：旧程序连新库，必须拒绝
	}
	for _, c := range cases {
		err := compareSchemaVersion(c.db, c.bin)
		if c.wantErr != (err != nil) {
			t.Fatalf("db=v%d bin=v%d: err=%v，期望报错=%v", c.db, c.bin, err, c.wantErr)
		}
	}
}

func TestCompareSchemaVersionMessageIsActionable(t *testing.T) {
	err := compareSchemaVersion(3, 2)
	if err == nil {
		t.Fatal("应返回错误")
	}
	msg := err.Error()
	// 报错必须说清「升级程序」或「恢复备份」，而不是只丢一个版本号
	if !strings.Contains(msg, "升级程序") || !strings.Contains(msg, "备份") {
		t.Fatalf("错误信息缺少可执行指引: %s", msg)
	}
}
