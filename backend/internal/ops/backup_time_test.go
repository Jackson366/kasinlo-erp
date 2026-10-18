package ops

import (
	"testing"
	"time"
)

// 备份时间必须取文件名里的时间戳：拷贝过的备份 mtime 会变成拷贝时间，
// 用它排序会把「新拷的旧备份」排到最前面，恢复时拿错文件。
// 这是全项目唯一一份解析实现（CLI 与 HTTP 列表共用），规则必须钉死。
func TestBackupTimeFromName(t *testing.T) {
	fallback := time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local)
	want := time.Date(2026, 10, 18, 22, 0, 0, 0, time.Local)

	// 库名无论用下划线还是横线分隔，都不能干扰：时间戳永远在末尾紧贴 .dump。
	for _, name := range []string{
		"nova_inv_v5-20261018220000.dump",
		"nova-inv-v5-20261018220000.dump",
	} {
		if got := BackupTimeFromName(name, fallback); !got.Equal(want) {
			t.Fatalf("BackupTimeFromName(%q) = %v, want %v", name, got, want)
		}
	}

	if got := BackupTimeFromName("weird-name.dump", fallback); !got.Equal(fallback) {
		t.Fatalf("解析不出时间戳应回落到 fallback，实际 %v", got)
	}
}
