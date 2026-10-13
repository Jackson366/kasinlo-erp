package test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"nova-erp/internal/database"
	"nova-erp/internal/model"
)

// 本文件覆盖第二批修复：「读漏了」型数据范围缺陷。
//
// 第一批堵的是「授权入口有旁路」（能不能授予）。
// 这一批堵的是「数据范围只在一部分读接口上生效」：
//
//	dept     5 个接口全无actor —— 部门树对任何管理员全量可见，还能改删别人的部门；
//	概览无范围               —— 「本部门」管理员看到的是全库人数与全公司操作量；
//	profile  半更新清空      —— 只改昵称会把邮箱/手机号/头像一并清空；
//	log      清空登录日志    —— 登录失败日志是账号锁定的计数来源，删掉等于解锁。
//
// 共同点：判定逻辑本身没错，缺的是「在哪些地方必须调用它」。

// scopedAdmin 造一个「技术部 + 系统管理员」的受限账号，返回令牌与用户 ID。
//
// 系统管理员的数据范围是「本部门及以下」，而部门树为：
//
//	1 总公司
//	├─ 2 技术部← 受限账号所在部门，范围内
//	└─ 3 运营部   ← 范围外
func scopedAdmin(t *testing.T, adminToken string, prefix string) (token string, userID int64) {
	t.Helper()
	suffix := time.Now().UnixNano()
	username := fmt.Sprintf("%s%d", prefix, suffix)
	create := call(t, http.MethodPost, "/api/v1/system/users", adminToken, map[string]any{
		"username": username, "password": "probe123456",
		"nickname": "范围受限探针", "deptId": 2, "roleIds": []int64{2},
	})
	assertCode(t, create, 0, "创建受限管理员 "+username)
	id := num(create.obj(t), "id")
	return loginAs(t, username, "probe123456"), id
}

// TestDeptListRespectsDataScope 部门树必须受数据范围约束（P2）。
//
// dept 模块此前 5 个接口全都没有 actor，部门树对任何管理员全量可见：
// 「本部门及以下」的部门管理员照样能看到运营部 —— 读漏了。
func TestDeptListRespectsDataScope(t *testing.T) {
	adminToken := loginAs(t, adminAccount, adminSecret)
	scopedToken, scopedID := scopedAdmin(t, adminToken, "dept_scope_")
	defer func() {
		call(t, http.MethodDelete, "/api/v1/system/users/"+itoa(scopedID), adminToken, nil)
	}()

	// 受限账号所在部门是 2（技术部），其子树不含 3（运营部）
	if !treeHasDept(t, scopedToken, 2) {
		t.Fatal("受限管理员应能看到本部门（2 技术部）")
	}
	if treeHasDept(t, scopedToken, 3) {
		t.Fatal("受限管理员的数据范围是「本部门及以下」，不应看到运营部（3）")
	}

	// 反向确认：超管看得到全部
	if !treeHasDept(t, adminToken, 3) {
		t.Fatal("超管应能看到运营部（3）")
	}
}

// TestDeptDetailAndWriteGuarded 详情与写接口必须同样收窄（P2）。
//
// 只过滤列表是不够的：「看不到但能改」依然是越权。
// 这里覆盖 Detail / Update / Delete 三条路径，越界一律按「不存在」返回，
// 不能用返回信息探测其他部门有哪些数据。
func TestDeptDetailAndWriteGuarded(t *testing.T) {
	adminToken := loginAs(t, adminAccount, adminSecret)
	scopedToken, scopedID := scopedAdmin(t, adminToken, "dept_write_")
	defer func() {
		call(t, http.MethodDelete, "/api/v1/system/users/"+itoa(scopedID), adminToken, nil)
	}()

	// 1. 详情：范围外按「不存在」
	detail := call(t, http.MethodGet, "/api/v1/system/depts/3", scopedToken, nil)
	assertStatus(t, detail, http.StatusNotFound, "读取范围外部门详情")
	assertCode(t, detail, 40400, "读取范围外部门详情")

	// 2. 修改：不能改别人的部门名
	update := call(t, http.MethodPut, "/api/v1/system/depts/3", scopedToken,
		map[string]any{"parentId": 1, "name": "运营部"})
	assertStatus(t, update, http.StatusForbidden, "修改范围外部门")
	assertCode(t, update, 40300, "修改范围外部门")

	// 3. 删除：不能删别人的部门
	del := call(t, http.MethodDelete, "/api/v1/system/depts/3", scopedToken, nil)
	assertStatus(t, del, http.StatusForbidden, "删除范围外部门")

	// 4. 不能把本部门挂到范围外的父部门下：
	//    这一步最隐蔽 —— 部门还在范围内，但挂上去之后它的数据范围实际已经变了。
	reparent := call(t, http.MethodPut, "/api/v1/system/depts/2", scopedToken,
		map[string]any{"parentId": 3, "name": "技术部"})
	assertStatus(t, reparent, http.StatusForbidden, "把部门挂到范围外的上级下")

	// 5. 新增部门时父部门也必须在范围内
	create := call(t, http.MethodPost, "/api/v1/system/depts", scopedToken,
		map[string]any{"parentId": 3, "name": "越权子部门"})
	assertStatus(t, create, http.StatusForbidden, "在范围外的父部门下新建部门")

	// 6. 确认运营部没被动过
	var name string
	if err := database.DB().Model(&model.Dept{}).Where("id = ?", 3).
		Select("name").Scan(&name).Error; err != nil {
		t.Fatalf("查询部门失败: %v", err)
	}
	if name != "运营部" {
		t.Fatalf("运营部被改动了，name=%q", name)
	}
}

// TestRecentLoginsRespectsDataScope 最近登录记录必须按数据范围过滤（P2）。
//
// 登录日志带用户名、IP、UserAgent，跨部门可见等于把
// 「谁在什么时候、从哪登录的」全量交出去，比人数泄露严重得多。
func TestRecentLoginsRespectsDataScope(t *testing.T) {
	adminToken := loginAs(t, adminAccount, adminSecret)
	scopedToken, scopedID := scopedAdmin(t, adminToken, "logins_scope_")
	defer func() {
		call(t, http.MethodDelete, "/api/v1/system/users/"+itoa(scopedID), adminToken, nil)
	}()

	// admin 在运营部之外（其部门未知），先确认超管能看到自己的登录记录
	full := call(t, http.MethodGet, "/api/v1/system/logs/logins?page=1&pageSize=10", adminToken, nil)
	assertCode(t, full, 0, "超管读取最近登录")
	if len(full.List) == 0 {
		t.Skip("测试库没有登录记录，跳过")
	}

	mine := call(t, http.MethodGet, "/api/v1/system/logs/logins?page=1&pageSize=10", scopedToken, nil)
	assertCode(t, mine, 0, "受限账号读取最近登录")
	// 受限账号刚登录过，至少应看到自己这一条
	if len(mine.List) == 0 {
		t.Fatal("受限账号应至少能看到自己的登录记录")
	}
	// 逐条核对：只允许出现「自己」或「技术部成员」的记录。
	// 允许集直接从库里取，避免测试里再实现一遍范围判定（那样就变成自证）。
	// 用 Unscoped 把软删用户也算进来：被删账号的历史登录行仍然留在日志表里，
	// 那不是越权（页面上已经查不到这个人），但若把它当成泄露会导致本用例随
	// 其他测试的执行顺序而随机失败。
	allowed := usersInDept(t, 2)
	var ownID int64
	if err := database.DB().Model(&model.User{}).Where("username = ?", "admin").
		Select("id").Scan(&ownID).Error; err != nil {
		t.Fatalf("查询 admin 失败: %v", err)
	}
	allowed[ownID] = true

	for _, item := range mine.List {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if uid := num(row, "userId"); !allowed[uid] {
			t.Fatalf("最近登录记录泄露了范围外的用户：%v (userId=%d)", row["username"], uid)
		}
	}
}

// usersInDept 返回该部门下的全部用户 ID 集合。
func usersInDept(t *testing.T, deptID int64) map[int64]bool {
	t.Helper()
	var ids []int64
	if err := database.DB().Unscoped().Model(&model.User{}).Where("dept_id = ?", deptID).
		Pluck("id", &ids).Error; err != nil {
		t.Fatalf("查询部门成员失败: %v", err)
	}
	out := make(map[int64]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

// TestProfilePartialUpdateKeepsFields 个人资料半更新不能清空字段（P2）。
//
// 与第一批 user.Update 的 dept_id 是同一类坑：GORM 对 map 形式的 Updates
// 不做零值省略。profile.Update 把四个字段全部写死进 map，
// 于是「只改昵称」的请求会把邮箱、手机号、头像一起清掉。
func TestProfilePartialUpdateKeepsFields(t *testing.T) {
	adminToken := loginAs(t, adminAccount, adminSecret)
	suffix := time.Now().UnixNano()
	username := fmt.Sprintf("prof_%d", suffix)

	create := call(t, http.MethodPost, "/api/v1/system/users", adminToken, map[string]any{
		"username": username, "password": "probe123456",
		"nickname": "半更新受害者", "email": "keep@example.com",
		"phone": "13800000001", "deptId": 2,
	})
	assertCode(t, create, 0, "创建用户")
	id := num(create.obj(t), "id")
	defer func() {
		call(t, http.MethodDelete, "/api/v1/system/users/"+itoa(id), adminToken, nil)
	}()

	token := loginAs(t, username, "probe123456")

	// 只带 nickname，邮箱 / 手机号都不带
	res := call(t, http.MethodPut, "/api/v1/profile", token,
		map[string]any{"nickname": "半更新受害者改"})
	assertCode(t, res, 0, "个人资料半更新")

	var email, phone string
	row := database.DB().Model(&model.User{}).Where("id = ?", id).
		Select("email", "phone").Row()
	if err := row.Scan(&email, &phone); err != nil {
		t.Fatalf("查询用户失败: %v", err)
	}
	if email != "keep@example.com" {
		t.Fatalf("未传 email 时邮箱被清空了，实际=%q", email)
	}
	if phone != "13800000001" {
		t.Fatalf("未传 phone 时手机号被清空了，实际=%q", phone)
	}

	// 反向确认：显式传空串即清空（PATCH 语义）
	clear := call(t, http.MethodPut, "/api/v1/profile", token,
		map[string]any{"nickname": "半更新受害者改", "email": ""})
	assertCode(t, clear, 0, "显式清空邮箱")

	if err := database.DB().Model(&model.User{}).Where("id = ?", id).
		Select("email").Scan(&email).Error; err != nil {
		t.Fatalf("查询用户失败: %v", err)
	}
	if email != "" {
		t.Fatalf("显式传空串应清空邮箱，实际=%q", email)
	}

	// 一个字段都不带应被明确拒绝，而不是静默成功
	empty := call(t, http.MethodPut, "/api/v1/profile", token, map[string]any{})
	assertStatus(t, empty, http.StatusBadRequest, "空更新请求")
}

// TestClearLoginsKeepsLockWindow 清空登录日志不得解除账号锁定（P2）。
//
// 登录失败日志不只是审计证据，它同时是 auth.isLocked 判定「连续失败多少次锁定」
// 的计数来源。若允许把它删掉，「清空登录日志」就等于「解除所有账号的锁定」——
// 攻击者只要能触发锁定，就能立刻恢复无限次尝试。
// 因此锁定期内的记录必须作为硬下界保留。
func TestClearLoginsKeepsLockWindow(t *testing.T) {
	adminToken := loginAs(t, adminAccount, adminSecret)

	// 先造一条「刚刚失败」的登录记录（admin 不在部门范围内，但这条只关心时间窗）
	probe := fmt.Sprintf("lockprobe_%d", time.Now().UnixNano())
	for i := 0; i < 2; i++ {
		call(t, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
			"username": probe, "password": "wrong-password",
		})
	}
	var recent int64
	if err := database.DB().Model(&model.LoginLog{}).
		Where("username = ? AND success = ?", probe, false).Count(&recent).Error; err != nil {
		t.Fatalf("统计失败登录记录失败: %v", err)
	}
	if recent == 0 {
		t.Skip("未能造出失败登录记录，跳过")
	}

	// 不带 before = 语义是「清空全部」，但锁定期内的记录必须留下
	res := call(t, http.MethodDelete, "/api/v1/system/logs/logins", adminToken, nil)
	assertCode(t, res, 0, "清空登录日志")

	var left int64
	if err := database.DB().Model(&model.LoginLog{}).
		Where("username = ? AND success = ?", probe, false).Count(&left).Error; err != nil {
		t.Fatalf("统计失败登录记录失败: %v", err)
	}
	if left == 0 {
		t.Fatal("清空登录日志把锁定期内的失败记录一起删了，等于解除账号锁定")
	}

	// 清理：把这条探针记录删掉（直接走库，绕过保护窗口）
	database.DB().Where("username = ?", probe).Delete(&model.LoginLog{})
}

// treeHasDept 判断部门树里是否出现指定部门 ID。
func treeHasDept(t *testing.T, token string, deptID int64) bool {
	t.Helper()
	res := call(t, http.MethodGet, "/api/v1/system/depts", token, nil)
	assertCode(t, res, 0, "读取部门树")
	return listHasDept(res.List, deptID)
}

func listHasDept(list []any, deptID int64) bool {
	for _, item := range list {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if num(row, "id") == deptID {
			return true
		}
		if kids, ok := row["children"].([]any); ok {
			if listHasDept(kids, deptID) {
				return true
			}
		}
	}
	return false
}
