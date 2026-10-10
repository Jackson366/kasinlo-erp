// 本文件收录「加固回归」用例：覆盖数据范围、令牌吊销、审计可信度这三类
// 编译期与常规 CRUD 测试都发现不了的缺陷。
//
// 与 regression_test.go 的分工：那里是「日志/导入导出」相关缺陷，
// 这里是与权限、身份、审计来源相关的缺陷。
package test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"nova-admin/internal/database"
	"nova-admin/internal/model"
	"nova-admin/internal/oplog"
)

// loginPair 登录并同时返回访问令牌与刷新令牌。
func loginPair(t *testing.T, account, secret string) (access, refresh string) {
	t.Helper()
	res := call(t, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"username": account, "password": secret,
	})
	if res.Code != 0 {
		t.Fatalf("账号 %s 登录失败: code=%d msg=%s", account, res.Code, res.Msg)
	}
	obj := res.obj(t)
	access = str(obj, "accessToken")
	refresh = str(obj, "refreshToken")
	if access == "" || refresh == "" {
		t.Fatalf("登录未返回完整令牌对: %v", obj)
	}
	return access, refresh
}

// loginWithUA 用指定 User-Agent 发起一次登录，返回业务响应。
func loginWithUA(eng *gin.Engine, account, secret, userAgent string) *response {
	body, _ := json.Marshal(map[string]any{"username": account, "password": secret})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	rec := serve(eng, req)

	res := &response{status: rec.Code}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		return res
	}
	res.Raw = payload
	if v, ok := payload["code"].(float64); ok {
		res.Code = int(v)
	}
	if v, ok := payload["message"].(string); ok {
		res.Msg = v
	}
	if d, ok := payload["data"].(map[string]any); ok {
		res.Data = d
	}
	return res
}

// clearDemoFailures 清掉 demo 的失败登录记录。
// 连续失败锁定是按「时间窗内的失败次数」判定的，测试残留会把账号锁死，
// 进而污染后续所有用例 —— 每个会制造失败登录的用例都必须自己收尾。
func clearDemoFailures() {
	database.DB().Where("username = ? AND success = ?", demoAccount, false).
		Delete(&model.LoginLog{})
}

// TestLoginLogSurvivesLongMultibyteUserAgent 复现并锁死「登录日志按字节截断 -> 锁定失效」。
//
// 缺陷成因：writeLoginLog 用 ua[:512] 截断。User-Agent 完全由客户端控制，
// 只要构造一个超过 512 字节、且第 512 字节落在多字节字符中间的 UA，
// 截断结果就是非法 UTF-8，PostgreSQL 以 22021 拒绝**整行**写入；
// 而错误被 `_ =` 吞掉，于是每一次失败尝试都不留痕迹。
// 由于 isLocked 正是按登录日志统计失败次数，锁定因此完全失效 —— 可无限暴力破解。
func TestLoginLogSurvivesLongMultibyteUserAgent(t *testing.T) {
	eng := setup(t)
	db := database.DB()

	clearDemoFailures()
	defer clearDemoFailures()

	// 511 个 ASCII + 一个 3 字节汉字 = 514 字节；按字节切到 512 恰好切坏汉字，
	// 但按字符数（512）正好在列上限内 —— 所以它必须能正常落库。
	evilUA := strings.Repeat("a", 511) + "中"
	if len(evilUA) <= 512 {
		t.Fatalf("构造的 UA 字节数应超过 512（否则不会触发截断），当前为 %d", len(evilUA))
	}
	if n := len([]rune(evilUA)); n > 512 {
		t.Fatalf("构造的 UA 字符数为 %d，已超出列上限，无法验证「截断后仍合法」", n)
	}

	// 阈值是 5，这里刻意多打一次
	for i := 0; i < 6; i++ {
		loginWithUA(eng, demoAccount, "definitely-wrong-password", evilUA)
	}

	var written int64
	if err := db.Model(&model.LoginLog{}).
		Where("username = ? AND success = ?", demoAccount, false).
		Count(&written).Error; err != nil {
		t.Fatalf("统计登录日志失败: %v", err)
	}
	if written < 6 {
		t.Fatalf("6 次失败登录只写入了 %d 条记录：登录日志被整行丢弃，锁定计数将无法累计", written)
	}

	var row model.LoginLog
	if err := db.Where("username = ?", demoAccount).Order("id DESC").First(&row).Error; err != nil {
		t.Fatalf("读取最新登录日志失败: %v", err)
	}
	if !utf8.ValidString(row.UserAgent) {
		t.Fatalf("落库的 User-Agent 不是合法 UTF-8（说明仍存在按字节截断）")
	}
	if n := len([]rune(row.UserAgent)); n > 512 {
		t.Fatalf("落库的 User-Agent 长度为 %d 字符，超过列上限 512", n)
	}

	// 关键断言：失败次数达到阈值后必须真的被锁定
	locked := loginWithUA(eng, demoAccount, "definitely-wrong-password", evilUA)
	if locked.Code != 40103 {
		t.Fatalf("连续 6 次失败后仍未被锁定：期望业务码 40103，实际 %d（msg=%s）",
			locked.Code, locked.Msg)
	}
}

// TestAuditIPIgnoresForwardedFor 审计日志里的来源 IP 不能被 X-Forwarded-For 伪造。
//
// gin 默认信任所有代理，会直接采用客户端可任意设置的 X-Forwarded-For，
// 于是操作日志与登录日志里的 IP 全是「客户端自报」，审计价值归零。
func TestAuditIPIgnoresForwardedFor(t *testing.T) {
	eng := setup(t)
	db := database.DB()

	const spoofed = "203.0.113.77"
	const probeUser = "ip_probe_user"

	db.Where("username = ?", probeUser).Delete(&model.LoginLog{})
	defer db.Where("username = ?", probeUser).Delete(&model.LoginLog{})

	body, _ := json.Marshal(map[string]any{"username": probeUser, "password": "whatever123"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", spoofed)
	req.Header.Set("User-Agent", "NovaHardeningTest/1.0")
	if rec := serve(eng, req); rec.Code == 0 {
		t.Fatal("伪造来源 IP 的登录请求没有得到响应")
	}

	var row model.LoginLog
	if err := db.Where("username = ?", probeUser).Order("id DESC").First(&row).Error; err != nil {
		t.Fatalf("未写入登录日志: %v", err)
	}
	if row.IP == spoofed {
		t.Fatalf("审计日志记录了 X-Forwarded-For 伪造的 IP %q：来源 IP 不可信", row.IP)
	}
	// httptest 的 RemoteAddr 固定为 192.0.2.1:1234，未信任代理时应取该地址
	if row.IP != "192.0.2.1" {
		t.Fatalf("期望记录 TCP 对端地址 192.0.2.1，实际 %q", row.IP)
	}
}

// TestPasswordChangeRevokesTokens 改密码后旧令牌（含刷新令牌）必须立即失效。
//
// 无状态 JWT 本身无法吊销，这里靠 sys_users.token_epoch 实现：
// 令牌携带签发时的版本号，改密码把版本号 +1，旧令牌在中间件处即被挡下。
func TestPasswordChangeRevokesTokens(t *testing.T) {
	setup(t)
	adminToken := loginAs(t, adminAccount, adminSecret)

	create := call(t, http.MethodPost, "/api/v1/system/users", adminToken, map[string]any{
		"username": "revoke_probe", "password": "probe123456",
		"nickname": "令牌吊销探针", "deptId": 2,
	})
	assertCode(t, create, 0, "创建探针用户")
	probeID := num(create.obj(t), "id")
	defer func() {
		call(t, http.MethodDelete, "/api/v1/system/users/"+itoa(probeID), adminToken, nil)
	}()

	access, refresh := loginPair(t, "revoke_probe", "probe123456")
	assertCode(t, call(t, http.MethodGet, "/api/v1/profile", access, nil), 0, "改密前旧令牌可用")

	// 管理员重置该用户密码
	reset := call(t, http.MethodPut, "/api/v1/system/users/"+itoa(probeID)+"/password",
		adminToken, map[string]any{"password": "probe654321"})
	assertCode(t, reset, 0, "重置密码")

	after := call(t, http.MethodGet, "/api/v1/profile", access, nil)
	assertStatus(t, after, http.StatusUnauthorized, "旧访问令牌应立即失效")
	assertCode(t, after, 40105, "旧访问令牌应立即失效")

	// 刷新令牌同样不能换出新的有效令牌，否则吊销只是把攻击者挡了 2 小时
	refreshRes := call(t, http.MethodPost, "/api/v1/auth/refresh", "", map[string]any{
		"refreshToken": refresh,
	})
	assertCode(t, refreshRes, 40105, "旧刷新令牌应立即失效")
}

// applyViewerDataScope 通过接口调整「只读访客」的数据范围。
// 走接口而不是直接改库，是为了让 authz 的身份缓存一并失效 —— 直接 UPDATE 不会失效缓存，
// 断言会在 30 秒的缓存窗口内读到旧身份，测试表现为「时好时坏」。
func applyViewerDataScope(t *testing.T, adminToken string, scope int16) {
	t.Helper()
	res := call(t, http.MethodPut, "/api/v1/system/roles/3", adminToken, map[string]any{
		"name": "只读访客", "sort": 3,
		"remark": "仅可查看列表与仪表盘，不可导出", "dataScope": scope,
	})
	assertCode(t, res, 0, "设置只读访客的数据范围")
}

// TestDataScopeIsEnforced 数据范围必须真正参与查询。
//
// 曾经的缺陷：data_scope 只被计算、入库、在角色表单里可配、还出现在导出文件里，
// 但没有任何一条查询用它 —— 配了「仅本人」的角色照样能列出并导出全部用户。
func TestDataScopeIsEnforced(t *testing.T) {
	setup(t)
	adminToken := loginAs(t, adminAccount, adminSecret)
	demoToken := loginAs(t, demoAccount, demoSecret)

	// 无论本用例怎么改，最后都还原成「全部」（与内置种子的取值一致）
	defer applyViewerDataScope(t, adminToken, model.DataScopeAll)

	// 基线：数据范围为「全部」时不应只有自己一条。
	// 这里显式设置一次，避免依赖库里当前恰好是什么值（老库可能是「仅本人」）。
	applyViewerDataScope(t, adminToken, model.DataScopeAll)
	unrestricted := call(t, http.MethodGet, "/api/v1/system/users?page=1&pageSize=20", demoToken, nil)
	assertCode(t, unrestricted, 0, "全部数据范围下列表")
	totalAll := num(unrestricted.obj(t), "total")
	if totalAll < 2 {
		t.Fatalf("前置条件不成立：期望至少 2 个用户，实际 %d", totalAll)
	}

	// 切成「仅本人」
	applyViewerDataScope(t, adminToken, model.DataScopeSelf)

	// 列表：只剩自己
	list := call(t, http.MethodGet, "/api/v1/system/users?page=1&pageSize=20", demoToken, nil)
	assertCode(t, list, 0, "仅本人范围下列表")
	if got := num(list.obj(t), "total"); got != 1 {
		t.Fatalf("「仅本人」应只看到 1 条用户，实际 %d 条 —— 数据范围未生效", got)
	}
	// 唯一可见的必须是自己（demo 的 id 为 2）
	if len(list.List) != 1 {
		t.Fatalf("「仅本人」的列表应有且仅有 1 条，实际 %d 条", len(list.List))
	}
	if row, ok := list.List[0].(map[string]any); !ok || num(row, "id") != 2 {
		t.Fatalf("「仅本人」看到的不是自己: %v", list.List[0])
	}

	// 详情：别人的记录按「不存在」处理，不泄露存在性
	detail := call(t, http.MethodGet, "/api/v1/system/users/1", demoToken, nil)
	assertStatus(t, detail, http.StatusNotFound, "越界查看用户详情")
	assertCode(t, detail, 40400, "越界查看用户详情")

	// 下拉选项同样受限
	options := call(t, http.MethodGet, "/api/v1/system/users/options", demoToken, nil)
	assertCode(t, options, 0, "仅本人范围下用户选项")
	if len(options.List) != 1 {
		t.Fatalf("「仅本人」的选项应只有 1 项，实际 %d 项", len(options.List))
	}
}

// TestDataScopeGatesWrites 数据范围必须同时约束写操作。
//
// 只收窄列表会留下「看不到但能改」的洞：受限管理员虽然列不出别的部门的人，
// 却仍可拿已知 ID 重置对方密码。这里用「本部门及以下」角色的真实账号验证。
func TestDataScopeGatesWrites(t *testing.T) {
	setup(t)
	adminToken := loginAs(t, adminAccount, adminSecret)

	// 建一个属于「技术部」(id=2) 且挂「系统管理员」(id=2，数据范围=本部门及以下) 的账号
	create := call(t, http.MethodPost, "/api/v1/system/users", adminToken, map[string]any{
		"username": "dept_admin_probe", "password": "probe123456",
		"nickname": "部门管理员探针", "deptId": 2, "roleIds": []int64{2},
	})
	assertCode(t, create, 0, "创建受限管理员")
	scopedID := num(create.obj(t), "id")
	defer func() {
		call(t, http.MethodDelete, "/api/v1/system/users/"+itoa(scopedID), adminToken, nil)
	}()

	scopedToken := loginAs(t, "dept_admin_probe", "probe123456")

	// 列表范围：只应看到技术部的人（demo 与它自己），不含总公司(id=1)的 admin
	list := call(t, http.MethodGet, "/api/v1/system/users?page=1&pageSize=50", scopedToken, nil)
	assertCode(t, list, 0, "本部门及以下范围下列表")
	for _, item := range list.List {
		row := item.(map[string]any)
		if row["id"].(float64) == 1 {
			t.Fatal("「本部门及以下」不应看到总公司（本人部门之外）的用户")
		}
	}

	// 导出范围必须与列表一致：越界的 ids 不能导出任何数据
	outOfScope := rawCall(t, http.MethodGet, "/api/v1/system/users/export?ids=1", scopedToken)
	if outOfScope.Code != http.StatusBadRequest {
		t.Fatalf("导出越界用户应无可导出数据（HTTP 400），实际 HTTP %d：%s",
			outOfScope.Code, outOfScope.Body.String())
	}
	inScope := rawCall(t, http.MethodGet, "/api/v1/system/users/export?ids=2", scopedToken)
	if inScope.Code != http.StatusOK {
		t.Fatalf("导出本部门用户应成功（HTTP 200），实际 HTTP %d", inScope.Code)
	}

	// 写操作：重置越界用户的密码必须失败，且按「不存在」处理
	reset := call(t, http.MethodPut, "/api/v1/system/users/1/password",
		scopedToken, map[string]any{"password": "hacked123456"})
	assertStatus(t, reset, http.StatusNotFound, "越界重置密码")
	assertCode(t, reset, 40400, "越界重置密码")

	// 反向确认：范围内的用户仍然可以正常操作
	okReset := call(t, http.MethodPut, "/api/v1/system/users/"+itoa(scopedID)+"/password",
		scopedToken, map[string]any{"password": "probe654321"})
	assertCode(t, okReset, 0, "范围内重置密码")
}

// TestOperationLogCoversUnmatchedRoute 未匹配到路由的写请求也要留下审计记录。
//
// 这同时是「操作日志中间件在引擎级、且位于 Recovery 之外」的回归锚点：
// 只有它的后置代码能拿到最终状态码，panic 的请求才不会在审计表里凭空消失。
func TestOperationLogCoversUnmatchedRoute(t *testing.T) {
	eng := setup(t)
	db := database.DB()

	const path = "/api/v1/system/no-such-endpoint"

	db.Where("path = ?", path).Delete(&model.OperationLog{})
	defer db.Where("path = ?", path).Delete(&model.OperationLog{})

	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader([]byte(`{"probe":true}`)))
	req.Header.Set("Content-Type", "application/json")
	serve(eng, req)

	if err := oplog.Flush(5 * time.Second); err != nil {
		t.Fatalf("刷新操作日志失败: %v", err)
	}

	var cnt int64
	if err := db.Model(&model.OperationLog{}).Where("path = ?", path).Count(&cnt).Error; err != nil {
		t.Fatalf("统计操作日志失败: %v", err)
	}
	if cnt == 0 {
		t.Fatal("未匹配到路由的写请求没有留下操作日志：审计中间件可能被放到了 Recovery 里层")
	}
}

// loginFromIP 从指定来源 IP 发起一次登录。
//
// httptest 默认的 RemoteAddr 固定为 192.0.2.1，要验证「按 账号+IP 分组」的锁定
// 就必须显式改写它。可信代理未配置时 ClientIP 取 TCP 对端地址，因此这里写入的 IP
// 就是服务端看到的来源 IP。
func loginFromIP(eng *gin.Engine, account, secret, ip string) *response {
	body, _ := json.Marshal(map[string]any{"username": account, "password": secret})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "NovaHardeningTest/ip-probe")
	req.RemoteAddr = ip + ":54321"
	rec := serve(eng, req)

	res := &response{status: rec.Code}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		return res
	}
	res.Raw = payload
	if v, ok := payload["code"].(float64); ok {
		res.Code = int(v)
	}
	if v, ok := payload["message"].(string); ok {
		res.Msg = v
	}
	if d, ok := payload["data"].(map[string]any); ok {
		res.Data = d
	}
	return res
}

// clearLoginsForIP 清理指定账号+IP 的登录日志。
func clearLoginsForIP(db *gorm.DB, username, ip string) {
	db.Where("username = ? AND ip = ?", username, ip).Delete(&model.LoginLog{})
}

// TestLoginLockoutIsScopedToAccountAndIP 连续失败锁定必须按「账号 + 来源 IP」计数。
//
// 旧实现只按 username 统计失败次数：
//
//  1. 任何人拿 5 次错密码就能把 admin 锁定 5 分钟 —— 锁号型 DoS 的成本几乎为零；
//  2. 登录成功并不清零，于是「失败 4 次 -> 成功 -> 失败 1 次」也会被莫名锁定。
func TestLoginLockoutIsScopedToAccountAndIP(t *testing.T) {
	eng := setup(t)
	db := database.DB()

	const attackerIP = "203.0.113.9" // 攻击者
	const victimIP = "198.51.100.5"  // 受害者的正常来源

	clearLoginsForIP(db, demoAccount, attackerIP)
	clearLoginsForIP(db, demoAccount, victimIP)
	defer func() {
		clearLoginsForIP(db, demoAccount, attackerIP)
		clearLoginsForIP(db, demoAccount, victimIP)
	}()

	// 攻击者用自己的 IP 连续失败（阈值 5，这里打 6 次）
	for i := 0; i < 6; i++ {
		loginFromIP(eng, demoAccount, "definitely-wrong-password", attackerIP)
	}
	// 攻击者自己被锁，符合预期
	if res := loginFromIP(eng, demoAccount, "definitely-wrong-password", attackerIP); res.Code != 40103 {
		t.Fatalf("攻击者 IP 应被锁定（40103），实际 %d", res.Code)
	}

	// 关键断言：受害者从自己的 IP 登录**不受影响**
	res := loginFromIP(eng, demoAccount, demoSecret, victimIP)
	if res.Code != 0 {
		t.Fatalf("他人失败不应锁死该账号：受害者登录期望成功，实际 code=%d msg=%s", res.Code, res.Msg)
	}

	// 成功登录必须清零失败计数：
	// 4 次失败 -> 成功 -> 再失败 1 次，不应被判为「连续失败达阈值」
	clearLoginsForIP(db, demoAccount, attackerIP)
	for i := 0; i < 4; i++ {
		loginFromIP(eng, demoAccount, "definitely-wrong-password", attackerIP)
	}
	if res := loginFromIP(eng, demoAccount, demoSecret, attackerIP); res.Code != 0 {
		t.Fatalf("第 4 次失败后成功登录应可用，实际 code=%d", res.Code)
	}
	res = loginFromIP(eng, demoAccount, "definitely-wrong-password", attackerIP)
	if res.Code != 40101 {
		t.Fatalf("成功登录后失败计数应重置：期望 40101（密码错误），实际 %d（%s）", res.Code, res.Msg)
	}
}

// TestRoleGrantCannotEscalate 角色分配必须做「可授予范围」校验，防止垂直越权。
//
// 旧实现里 replaceRoles 只过滤「不存在的角色」，于是拥有 system:user:create 的
// 系统管理员可以带 roleIds:[1]（超管）建号，或把自己/下属改成超管 —— 一次请求提权。
func TestRoleGrantCannotEscalate(t *testing.T) {
	setup(t)
	adminToken := loginAs(t, adminAccount, adminSecret)

	// 造一个「技术部 + 系统管理员(role 2, 数据范围=本部门及以下)」的受限管理员
	create := call(t, http.MethodPost, "/api/v1/system/users", adminToken, map[string]any{
		"username": "esc_probe_admin", "password": "probe123456",
		"nickname": "越权探针管理员", "deptId": 2, "roleIds": []int64{2},
	})
	assertCode(t, create, 0, "创建受限管理员")
	scopedID := num(create.obj(t), "id")

	// 由它创建一个普通用户，用于最后验证「合法授予仍可正常工作」
	okUser := call(t, http.MethodPost, "/api/v1/system/users", adminToken, map[string]any{
		"username": "esc_probe_user", "password": "probe123456",
		"nickname": "越权探针用户", "deptId": 2,
	})
	assertCode(t, okUser, 0, "创建探针用户")
	okUserID := num(okUser.obj(t), "id")

	defer func() {
		call(t, http.MethodDelete, "/api/v1/system/users/"+itoa(scopedID), adminToken, nil)
		call(t, http.MethodDelete, "/api/v1/system/users/"+itoa(okUserID), adminToken, nil)
	}()

	scopedToken := loginAs(t, "esc_probe_admin", "probe123456")

	// 1) 不能新建一个超管账号
	escalate := call(t, http.MethodPost, "/api/v1/system/users", scopedToken, map[string]any{
		"username": "esc_probe_super", "password": "probe123456",
		"nickname": "提权尝试", "deptId": 2, "roleIds": []int64{1},
	})
	assertStatus(t, escalate, http.StatusForbidden, "授予超管角色")
	assertCode(t, escalate, 40300, "授予超管角色")

	// 2) 也不能把自己改成超管
	selfEscalate := call(t, http.MethodPut, "/api/v1/system/users/"+itoa(scopedID), scopedToken,
		map[string]any{"nickname": "越权探针管理员", "deptId": 2, "roleIds": []int64{1, 2}})
	assertStatus(t, selfEscalate, http.StatusForbidden, "自我提权")
	assertCode(t, selfEscalate, 40300, "自我提权")

	// 3) 角色下拉不应出现超管角色（否则是一个「选了必然 403」的选项）
	options := call(t, http.MethodGet, "/api/v1/system/roles/options", scopedToken, nil)
	assertCode(t, options, 0, "受限管理员拉取角色下拉")
	for _, item := range options.List {
		row := item.(map[string]any)
		if row["id"].(float64) == 1 {
			t.Fatal("角色下拉不应包含无权重授予的超级管理员角色")
		}
	}

	// 4) 反向确认：权限未超出的角色仍可正常授予
	grant := call(t, http.MethodPut, "/api/v1/system/users/"+itoa(okUserID), scopedToken,
		map[string]any{"nickname": "越权探针用户", "deptId": 2, "roleIds": []int64{3}})
	assertCode(t, grant, 0, "授予范围内角色")

	// 5) 提权尝试全部失败后，超管账号不应真的被造出来
	var cnt int64
	if err := database.DB().Model(&model.User{}).
		Where("username = ?", "esc_probe_super").Count(&cnt).Error; err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	if cnt != 0 {
		t.Fatal("被拒绝的提权请求仍然写入了用户记录")
	}
}

// TestTraceIDRejectsInjectedHeader 客户端传入的 X-Trace-Id 必须做字符集收敛。
//
// 只限长度是不够的：该值会被原样写进响应头与日志，含 CRLF 或控制字符时
// 可用于响应头拆分、日志注入。
func TestTraceIDRejectsInjectedHeader(t *testing.T) {
	eng := setup(t)

	cases := []struct {
		name string
		in   string
		keep bool
	}{
		{"合法字符原样透传", "abc-123_XYZ", true},
		{"含 CRLF 应重新生成", "abc\r\nX-Evil: 1", false},
		{"含空格应重新生成", "abc def", false},
		{"超长应重新生成", strings.Repeat("a", 65), false},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.Header.Set("X-Trace-Id", tc.in)
		rec := serve(eng, req)
		got := rec.Header().Get("X-Trace-Id")
		if got == "" {
			t.Fatalf("%s: 响应应始终带 X-Trace-Id", tc.name)
		}
		if tc.keep && got != tc.in {
			t.Fatalf("%s: 期望原样透传 %q，实际 %q", tc.name, tc.in, got)
		}
		if !tc.keep && got == tc.in {
			t.Fatalf("%s: 非法值不应被原样透传", tc.name)
		}
	}
}
