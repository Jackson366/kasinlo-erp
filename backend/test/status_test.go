package test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"nova-erp/internal/database"
	"nova-erp/internal/model"
)

// statusCase 描述「新增后回读状态」这一族用例。
type statusCase struct {
	label  string // 子测试名
	path   string // 资源根路径，如 /system/users
	table  string // 数据库表名
	column string // 用于定位记录的列（与生成的名字同值）
	// filterByStatus 表示该列表接口支持 &status= 过滤。
	// 部门与菜单返回的是完整树（需要展示已停用节点才能重新启用），角色列表也不带该筛选，
	// 只有用户列表支持，因此按状态筛选只对用户断言。
	filterByStatus bool
	body           func(name string) map[string]any
}

// assertDBStatus 直接查库确认 status 的真实取值。
//
// 必须绕开接口回读：Handler 返回的是 VO，万一 VO 层做了兜底赋值，
// 只看接口响应会把「落库就是错的」这类缺陷重新掩盖起来。
func assertDBStatus(t *testing.T, table, column, name string, want int16) {
	t.Helper()

	var got int16
	err := database.DB().Table(table).Where(column+" = ?", name).Select("status").Scan(&got).Error
	if err != nil {
		t.Fatalf("查询 %s.status 失败: %v", table, err)
	}
	if got != want {
		t.Fatalf("%s 中 %s=%s 的 status 期望 %d，实际 %d", table, column, name, want, got)
	}
}

// TestDisabledStatusPersists 回归测试：显式选择「禁用」的记录必须真的以 status=0 落库。
//
// 这个缺陷的形态极其隐蔽：GORM 对「带 default 标签、且当前值为零值」的字段，
// 会把该列整个从 INSERT 语句里省略掉，转而使用数据库默认值。
// 而 model.StatusDisabled 恰好等于 0 —— 也就是 Go 的零值 ——
// 于是「禁用」被静默写成了数据库默认值 1（启用）：
// 编译通过、类型检查通过、接口返回 200、常规 CRUD 测试也发现不了，
// 只有真正去读 status 字段才会暴露。
//
// 因此本测试同时覆盖两条判定路径：数据库真实值 + 接口回读值，
// 并对「启用」保留一个对照组，防止测试本身退化成恒真断言。
func TestDisabledStatusPersists(t *testing.T) {
	token := loginAs(t, adminAccount, adminSecret)
	suffix := time.Now().UnixNano()

	cases := []statusCase{
		{
			label: "用户", path: "/system/users", table: "sys_users", column: "username", filterByStatus: true,
			body: func(name string) map[string]any {
				return map[string]any{
					"username": name, "password": "abc123456", "nickname": "状态校验", "status": 0,
				}
			},
		},
		{
			label: "部门", path: "/system/depts", table: "sys_depts", column: "name",
			body: func(name string) map[string]any {
				return map[string]any{"name": name, "parentId": 1, "status": 0}
			},
		},
		{
			label: "角色", path: "/system/roles", table: "sys_roles", column: "code",
			body: func(name string) map[string]any {
				return map[string]any{"name": name, "code": name, "dataScope": 1, "status": 0}
			},
		},
		{
			label: "菜单", path: "/system/menus", table: "sys_menus", column: "name",
			body: func(name string) map[string]any {
				return map[string]any{"name": name, "title": "状态校验", "type": "button", "status": 0}
			},
		},
	}

	for i, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			// 每个模块用两套名字，分别验证「禁用」与「启用」两个分支
			disabledName := fmt.Sprintf("stchk%d_%d_off", suffix, i)
			enabledName := fmt.Sprintf("stchk%d_%d_on", suffix, i)

			// ---- 分支一：显式禁用（status=0）----
			disabledBody := tc.body(disabledName)
			disabledBody["status"] = 0
			res := call(t, http.MethodPost, "/api/v1"+tc.path, token, disabledBody)
			assertCode(t, res, 0, tc.label+"新增禁用记录")
			disabledID := num(res.obj(t), "id")
			if disabledID == 0 {
				t.Fatalf("%s：新增未返回记录 ID", tc.label)
			}
			defer call(t, http.MethodDelete, fmt.Sprintf("/api/v1%s/%d", tc.path, disabledID), token, nil)

			assertDBStatus(t, tc.table, tc.column, disabledName, model.StatusDisabled)

			detail := call(t, http.MethodGet, fmt.Sprintf("/api/v1%s/%d", tc.path, disabledID), token, nil)
			assertCode(t, detail, 0, tc.label+"查询禁用记录详情")
			if got := num(detail.obj(t), "status"); got != int64(model.StatusDisabled) {
				t.Fatalf("%s：详情回读 status 期望 %d，实际 %d", tc.label, model.StatusDisabled, got)
			}

			// 禁用记录必须能被列表按 status=0 筛出来，否则「按状态筛选」也是坏的
			if tc.filterByStatus {
				list := call(t, http.MethodGet, "/api/v1"+tc.path+"?status=0", token, nil)
				assertCode(t, list, 0, tc.label+"按禁用状态筛选")
				if !contains(fmt.Sprint(list.List), disabledName) {
					t.Fatalf("%s：按 status=0 筛选未包含刚新增的禁用记录 %s", tc.label, disabledName)
				}
			}

			// ---- 分支二：显式启用（status=1）作为对照组 ----
			enabledBody := tc.body(enabledName)
			enabledBody["status"] = 1
			res = call(t, http.MethodPost, "/api/v1"+tc.path, token, enabledBody)
			assertCode(t, res, 0, tc.label+"新增启用记录")
			enabledID := num(res.obj(t), "id")
			defer call(t, http.MethodDelete, fmt.Sprintf("/api/v1%s/%d", tc.path, enabledID), token, nil)

			assertDBStatus(t, tc.table, tc.column, enabledName, model.StatusEnabled)
		})
	}
}

// TestImportDisabledStatusPersists 验证导入路径同样不会把「禁用」写成「启用」。
//
// 导入走的是 CreateInBatches，与单条 Create 是两条不同的代码路径，
// 必须各自覆盖 —— 只修一处的话，另一处会以完全相同的方式静默出错。
func TestImportDisabledStatusPersists(t *testing.T) {
	token := loginAs(t, adminAccount, adminSecret)
	suffix := time.Now().UnixNano()

	offUser := fmt.Sprintf("impoff%d", suffix)
	onUser := fmt.Sprintf("impon%d", suffix)
	blankUser := fmt.Sprintf("impblank%d", suffix)

	rows := [][]string{
		{offUser, "导入禁用", "", "", "", "", "", "禁用", "状态校验"},
		{onUser, "导入启用", "", "", "", "", "", "启用", "状态校验"},
		// 状态留空：按约定应视为「启用」
		{blankUser, "导入留空", "", "", "", "", "", "", "状态校验"},
	}

	rec := uploadCall(t, "/api/v1/system/users/import", token, "users.xlsx", buildSheet(t, importHeaders, rows))
	if rec.Code != http.StatusOK {
		t.Fatalf("导入失败: HTTP %d body=%s", rec.Code, rec.Body.String())
	}
	result, code := parseImportResult(t, rec.Body.Bytes())
	if code != 0 || result.Created != 3 {
		t.Fatalf("期望导入 3 条，实际 created=%d code=%d（valid=%d invalid=%d）",
			result.Created, code, result.Valid, result.Invalid)
	}

	assertDBStatus(t, "sys_users", "username", offUser, model.StatusDisabled)
	assertDBStatus(t, "sys_users", "username", onUser, model.StatusEnabled)
	assertDBStatus(t, "sys_users", "username", blankUser, model.StatusEnabled)

	// 清理导入产生的数据，避免影响后续用例（软删除即可，用户名唯一性只看存活记录）
	for _, name := range []string{offUser, onUser, blankUser} {
		var u model.User
		if err := database.DB().Where("username = ?", name).First(&u).Error; err != nil {
			t.Fatalf("回查导入用户 %s 失败: %v", name, err)
		}
		call(t, http.MethodDelete, "/api/v1/system/users/"+itoa(u.ID), token, nil)
	}
}
