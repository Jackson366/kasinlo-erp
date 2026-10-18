package ops

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// 文件名带路径就能越出 staging 目录写文件 —— 升级包是上传来的，不可信输入。
func TestValidateManifestRejectsPathTraversal(t *testing.T) {
	cases := []struct {
		name string
		mf   upgradeManifest
	}{
		{"反斜杠", upgradeManifest{Version: "1.0.0", Backend: &upgradeFile{File: `..\evil.exe`, SHA256: "x"}}},
		{"上级目录", upgradeManifest{Version: "1.0.0", Backend: &upgradeFile{File: "../evil.exe", SHA256: "x"}}},
		{"嵌套路径", upgradeManifest{Version: "1.0.0", Backend: &upgradeFile{File: "bin/nova-erp", SHA256: "x"}}},
		{"裸点", upgradeManifest{Version: "1.0.0", Backend: &upgradeFile{File: ".", SHA256: "x"}}},
		{"缺摘要", upgradeManifest{Version: "1.0.0", Backend: &upgradeFile{File: "nova-erp"}}},
		{"缺文件名", upgradeManifest{Version: "1.0.0", Frontend: &upgradeFile{SHA256: "x"}}},
	}
	for _, c := range cases {
		if err := validateManifest(&c.mf); err == nil {
			t.Fatalf("%s：应拒绝 %q", c.name, c.mf.Backend.File)
		}
	}
}

func TestValidateManifestRequiresVersion(t *testing.T) {
	if err := validateManifest(&upgradeManifest{Backend: &upgradeFile{File: "a", SHA256: "b"}}); err == nil {
		t.Fatal("缺少 version 必须报错")
	}
	// 版本号里带路径分隔符会让它变成文件名的一部分，必须拦
	if err := validateManifest(&upgradeManifest{Version: "../1.0", Backend: &upgradeFile{File: "a", SHA256: "b"}}); err == nil {
		t.Fatal("version 含路径分隔符必须报错")
	}
}

func TestValidateManifestAcceptsBackendOnly(t *testing.T) {
	// 只更后端的补丁包很常见，不该被强制要求带前端
	if err := validateManifest(&upgradeManifest{Version: "1.0.0", MinSchemaVersion: 1,
		Backend: &upgradeFile{File: "nova-erp", SHA256: "abc"}}); err != nil {
		t.Fatalf("仅含后端的包应通过：%v", err)
	}
}

// 嵌套目录要么说明打包方式不对，要么是想借路径做别的事，两种都该拒绝。
func TestReadZipFilesRejectsNestedDirs(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("manifest.json")
	_, _ = w.Write([]byte("{}"))
	w2, _ := zw.Create("sub/nova-erp")
	_, _ = w2.Write([]byte("x"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := readZipFiles(zr); err == nil {
		t.Fatal("包含嵌套目录的升级包应被拒绝")
	}
}

func TestReadZipFilesSkipsDirs(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("manifest.json")
	_, _ = w.Write([]byte("{}"))
	if _, err := zw.Create("dist/"); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	files, err := readZipFiles(zr)
	if err != nil {
		t.Fatalf("目录项应被跳过而不是报错：%v", err)
	}
	if len(files) != 1 {
		t.Fatalf("应只收到 1 个文件，实际 %d", len(files))
	}
}

// 同一份包重复上传必须得到同一个计划 ID，否则 staging 目录会无限增生。
func TestNewPlanIDStable(t *testing.T) {
	a := newPlanID("abcdef0123456789", "1.2.0")
	b := newPlanID("abcdef0123456789", "1.2.0")
	if a != b {
		t.Fatalf("同一输入应得到同一 ID：%s vs %s", a, b)
	}
	if !strings.HasPrefix(a, "upg-1.2.0-") {
		t.Fatalf("ID 应带版本前缀：%s", a)
	}
	if c := newPlanID("ffffffffffffffff", "1.2.0"); c == a {
		t.Fatal("不同摘要应得到不同 ID")
	}
}

// 动作清单要能原样往返：执行后逐项回报结果靠的就是这段 JSON。
func TestPlanJSONRoundTrip(t *testing.T) {
	actions := []UpgradeAction{
		{Key: "backend", Title: "替换后端程序", Detail: "a → b", Done: true},
		{Key: "frontend", Title: "替换前端产物", Detail: "c → d", Done: false, Error: "权限不足"},
	}
	raw := planJSONFromActions(actions)
	var back []UpgradeAction
	if err := json.Unmarshal([]byte(raw), &back); err != nil {
		t.Fatal(err)
	}
	if len(back) != 2 || back[1].Error != "权限不足" || !back[0].Done {
		t.Fatalf("往返结果不符：%+v", back)
	}
}
