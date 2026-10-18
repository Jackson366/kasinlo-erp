package ops

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
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
func TestOpenPackageRejectsNestedDirs(t *testing.T) {
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
	if _, err := openPackage(zr); err == nil {
		t.Fatal("包含嵌套目录的升级包应被拒绝")
	}
}

func TestOpenPackageSkipsDirsAndSeparatesManifest(t *testing.T) {
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
	pkg, err := openPackage(zr)
	if err != nil {
		t.Fatalf("目录项应被跳过而不是报错：%v", err)
	}
	if pkg.manifest == nil {
		t.Fatal("manifest.json 应被单独读入内存")
	}
	if len(pkg.files) != 0 {
		t.Fatalf("manifest 不应算进 files，实际 %d", len(pkg.files))
	}
}

// newPlanID 含秒级时间戳是**有意**的：同一份包不同时间上传得到不同 ID，
// 允许失败后重传（PlanID 是唯一索引，去掉时间戳会让同包重传撞索引）。
// 真正的「同包重复上传」防护在 ParseUpgradePackage 里按 package_sha256 拒绝 pending。
func TestNewPlanIDFormatAndUniqueness(t *testing.T) {
	a := newPlanID("abcdef0123456789", "1.2.0")
	if !strings.HasPrefix(a, "upg-1.2.0-") {
		t.Fatalf("ID 应带版本前缀：%s", a)
	}
	// 不同摘要得到不同 ID
	if c := newPlanID("ffffffffffffffff", "1.2.0"); c == a {
		t.Fatal("不同摘要应得到不同 ID")
	}
	// ID 以 14 位时间戳结尾：靠它区分「同包的不同次上传」
	ts := a[len(a)-14:]
	if _, err := time.Parse("20060102150405", ts); err != nil {
		t.Fatalf("ID 应以 14 位时间戳结尾：%s", a)
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

// 纯前端包执行成功即升级完成：前端目录切换是即时生效的，
// 若按后端包的流程「等重启比对版本」，二进制永远不变（它不在包里），
// 记录会被永久误标为「未生效」。
func TestUpgradeOutcomeFrontendOnlyIsSuccess(t *testing.T) {
	status, msg := upgradeOutcome([]UpgradeAction{
		{Key: "frontend", Title: "替换前端产物", Done: true},
	}, "")
	if status != "success" {
		t.Fatalf("纯前端包应直接 success，实际 %s", status)
	}
	if msg == "" || !strings.Contains(msg, "无需重启") {
		t.Fatalf("说明应写明无需重启：%s", msg)
	}
}

func TestUpgradeOutcomeWithBackendWaitsRestart(t *testing.T) {
	status, _ := upgradeOutcome([]UpgradeAction{
		{Key: "backend", Title: "替换后端程序", Done: true},
		{Key: "frontend", Title: "替换前端产物", Done: true},
	}, "")
	if status != "applied" {
		t.Fatalf("含后端动作应等待重启（applied），实际 %s", status)
	}
}

func TestUpgradeOutcomeFailureWins(t *testing.T) {
	status, msg := upgradeOutcome([]UpgradeAction{
		{Key: "backend", Done: false, Error: "磁盘不足"},
	}, "磁盘不足")
	if status != "failed" || !strings.Contains(msg, "磁盘不足") {
		t.Fatalf("执行失败应优先于其它判定：%s / %s", status, msg)
	}
}
