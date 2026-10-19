package ops

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"nova-erp/internal/pkg/errcode"
)

// locateTool 找到 pg_dump / pg_restore 的可执行文件。
//
// 为什么要探测而不是直接用 `pg_dump`：Windows 上 PostgreSQL 装完**默认不把 bin
// 加进 PATH**，运维照文档敲 `pg_dump` 得到的是「不是内部或外部命令」。
// 与其让人去手改环境变量，不如在这里按常见安装位置找一遍，并把找到的路径
// 打进日志（出问题时能立刻确认用的是哪个版本）。
//
// 匹配到多个版本时取版本号最大的：机器上同时装了 16 和 17 很常见，
// 用低版本 pg_dump 备份高版本库会直接报「server version mismatch」。
func locateTool(name, configured string) (string, error) {
	if configured != "" {
		if _, err := os.Stat(configured); err != nil {
			return "", errcode.Newf(errcode.ErrBadRequest, "配置的 %s 路径不可用: %s: %v", name, configured, err)
		}
		return configured, nil
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}

	exe := name
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	patterns := []string{
		filepath.Join(os.Getenv("ProgramFiles"), "PostgreSQL", "*", "bin", exe),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "PostgreSQL", "*", "bin", exe),
		"/usr/lib/postgresql/*/bin/" + name,
		"/usr/pgsql-*/bin/" + name,
		"/opt/homebrew/opt/postgresql@*/bin/" + name,
		"/usr/local/opt/postgresql@*/bin/" + name,
		"/Applications/Postgres.app/Contents/Versions/*/bin/" + name,
	}

	var found []string
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		for _, m := range matches {
			if st, err := os.Stat(m); err == nil && !st.IsDir() {
				found = append(found, m)
			}
		}
	}
	if len(found) == 0 {
		return "", errcode.Newf(errcode.ErrBadRequest,
			"找不到 %s：本机没有安装 PostgreSQL 客户端，或安装路径未被识别。请安装 PostgreSQL，或在 config.yaml 的 backup.%s_path 填写 pg_dump/pg_restore 的绝对路径",
			name, strings.ReplaceAll(name, "_", ""))
	}
	sort.Slice(found, func(i, j int) bool { return versionLess(found[j], found[i]) })
	return found[0], nil
}

// versionLess 比较两个路径里的版本号（如 PostgreSQL/16 与 /17、postgresql@14 与 @15）。
//
// 按路径里的数字段逐段比较，纯字符串比较会把 9 排在 10 前面 —— 机器上同时留着
// PostgreSQL 9.6 与 17 时，就会挑中那个读不了新库的老客户端。
func versionLess(a, b string) bool {
	va, vb := versionParts(a), versionParts(b)
	for i := 0; i < len(va) && i < len(vb); i++ {
		if va[i] != vb[i] {
			return va[i] < vb[i]
		}
	}
	return len(va) < len(vb)
}

func versionParts(path string) []int {
	var out []int
	for _, seg := range strings.FieldsFunc(path, func(r rune) bool {
		return !(r >= '0' && r <= '9')
	}) {
		if n, err := strconv.Atoi(seg); err == nil {
			out = append(out, n)
		}
	}
	return out
}
