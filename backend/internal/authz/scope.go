package authz

import (
	"gorm.io/gorm"

	"nova-erp/internal/database"
	"nova-erp/internal/model"
	"nova-erp/internal/pkg/errcode"
)

// ---------------- 数据范围（数据权限） ----------------
//
// 数据范围来自角色上的 data_scope，多个角色取最宽松者（见 build）：
//
//	1 全部         不加限制
//	2 本部门       仅本部门
//	3 本部门及以下 本部门及其所有子部门
//	4 仅本人       只有自己
//
// 本文件把这套口径收敛成唯一实现。此前 user / dept / dashboard 三个模块
// 各自零散地处理数据范围，dept 与 dashboard 干脆完全不处理——
// 于是「本部门」管理员虽然列不出别的部门的人，却能在部门树上看到全公司结构、
// 在仪表盘上看到全库人数，这属于典型的「读漏了」型越权。
//
// 两条通用原则：
//  1. 读与写必须一起收窄。写接口要调 EnsureDeptWritable，
//     否则「看不到但能改/能删」的洞依旧存在。
//  2. 越界一律按「不存在」处理（404），避免用返回信息探测其他部门有哪些数据。

// Scope 是一次身份的数据范围解析结果。
type Scope struct {
	// Unrestricted 为 true 表示不受限（超管、未认证身份、全部数据范围）。
	Unrestricted bool
	// SelfOnly 为 true 表示「仅本人」。
	SelfOnly bool
	// UserID 是 SelfOnly 时的可见用户 ID。
	UserID int64
	// DeptIDs 是可见部门集合；Unrestricted / SelfOnly 时为 nil。
	DeptIDs []int64
}

// ResolveScope 解析 actor 的数据范围。
//
// actor 为 nil（未认证）或超管 → 不受限；数据范围为「全部」同样不受限。
func ResolveScope(actor *Identity) *Scope {
	if actor == nil || actor.IsSuper || actor.DataScope == model.DataScopeAll {
		return &Scope{Unrestricted: true}
	}
	if actor.DataScope == model.DataScopeSelf || actor.DeptID == nil {
		// 没有部门的人谈不上「本部门」，退化成只能看自己
		return &Scope{SelfOnly: true, UserID: actor.UserID}
	}
	root := *actor.DeptID
	ids := []int64{root}
	if actor.DataScope == model.DataScopeDeptAndSub {
		var err error
		if ids, err = DeptTreeIDs(root); err != nil {
			// 取部门树失败时退回「仅本部门」：比返回全量安全得多，
			// 代价只是子部门数据暂时看不到。
			ids = []int64{root}
		}
	}
	return &Scope{DeptIDs: ids}
}

// AllowsDept 判断部门是否落在范围内。
func (s *Scope) AllowsDept(id int64) bool {
	if s.Unrestricted {
		return true
	}
	if s.SelfOnly {
		return false
	}
	for _, d := range s.DeptIDs {
		if d == id {
			return true
		}
	}
	return false
}

// DeptIn 返回「部门 ID IN (...)」的 SQL 片段与参数；不受限时 ok=false。
func (s *Scope) DeptIn(alias string) (string, []any, bool) {
	if s.Unrestricted {
		return "", nil, false
	}
	if s.SelfOnly {
		// 「仅本人」没有可枚举的部门集合，交由调用方按用户处理
		return "", nil, false
	}
	col := "id"
	if alias != "" {
		col = alias + ".id"
	}
	return col + " IN ?", []any{s.DeptIDs}, true
}

// UserCond 返回作用于 sys_users 表（含可选别名）的范围 where 片段。
//
// ok=false 表示不限，调用方不要追加任何条件。
// 别名必须与 GORM 的 Model/Table 使用的一致，否则列名解析不到。
func (s *Scope) UserCond(alias string) (string, []any, bool) {
	if s.Unrestricted {
		return "", nil, false
	}
	p := func(col string) string {
		if alias == "" {
			return col
		}
		return alias + "." + col
	}
	if s.SelfOnly {
		return p("id") + " = ?", []any{s.UserID}, true
	}
	return p("dept_id") + " IN ?", []any{s.DeptIDs}, true
}

// LogUserCond 返回作用于「带 user_id 列的表」（登录日志 / 操作日志）的范围片段。
//
// 之所以不能直接套 UserCond：日志表上的 user_id 可能为 0（未识别出账号的失败登录），
// 这类记录不属于任何人的数据范围，非超管一律看不到。
func (s *Scope) LogUserCond() (string, []any, bool) {
	if s.Unrestricted {
		return "", nil, false
	}
	if s.SelfOnly {
		return "user_id = ?", []any{s.UserID}, true
	}
	return "user_id IN (SELECT id FROM sys_users WHERE dept_id IN ?)", []any{s.DeptIDs}, true
}

// CreatorCond 返回作用于「带 created_by 列的表」（进销存单据）的范围片段。
//
// 与 LogUserCond 同构，只是列名是 created_by。进销存单据按「谁创建」收窄：
// 本部门 = 创建人所在部门，仅本人 = 创建人就是自己。
//
// 注意：只对**单据**（采购/销售/出入库/结算）套这个条件。商品 / 分类 / 客户 /
// 供应商是**共享主数据**，没有「创建人」语义 —— 隔离了会导致开单时选不到
// 别人维护的商品与客户，业务直接断掉。它们仍由权限码控制谁能改，只是不看数据范围。
func (s *Scope) CreatorCond(alias string) (string, []any, bool) {
	if s.Unrestricted {
		return "", nil, false
	}
	col := "created_by"
	if alias != "" {
		col = alias + ".created_by"
	}
	if s.SelfOnly {
		return col + " = ?", []any{s.UserID}, true
	}
	return col + " IN (SELECT id FROM sys_users WHERE dept_id IN ?)", []any{s.DeptIDs}, true
}

// CreatorVisible 校验某条单据的创建人是否落在数据范围内。
//
// 用于**写路径**（改/删/审核/作废/结算）与需要精确判断的单条读：越界按
// 「不存在」处理（404），避免用返回信息探测其他部门有哪些单据。
func (s *Scope) CreatorVisible(creatorID int64) (bool, error) {
	if s.Unrestricted {
		return true, nil
	}
	if s.SelfOnly {
		return creatorID == s.UserID, nil
	}
	var deptID int64
	if err := database.DB().Table("sys_users").
		Select("dept_id").Where("id = ?", creatorID).Scan(&deptID).Error; err != nil {
		return false, errcode.Wrap(errcode.ErrDatabase, err)
	}
	for _, d := range s.DeptIDs {
		if d == deptID {
			return true, nil
		}
	}
	return false, nil
}

// ApplyUser 把范围条件叠在 sys_users 查询上。
func (s *Scope) ApplyUser(tx *gorm.DB) *gorm.DB {
	cond, args, ok := s.UserCond("")
	if !ok {
		return tx
	}
	return tx.Where(cond, args...)
}

// ApplyLog 把范围条件叠在带 user_id 的日志查询上。
func (s *Scope) ApplyLog(tx *gorm.DB) *gorm.DB {
	cond, args, ok := s.LogUserCond()
	if !ok {
		return tx
	}
	return tx.Where(cond, args...)
}

// EnsureDeptVisible 校验部门是否可见；越界按「不存在」返回。
func (s *Scope) EnsureDeptVisible(id int64) error {
	if s.AllowsDept(id) {
		return nil
	}
	return errcode.New(errcode.ErrNotFound, "部门不存在")
}

// EnsureDeptWritable 校验部门是否可写。
//
// 注意与 EnsureDeptVisible 的**返回码刻意不同**：可见性是「读漏了」型越权，
// 按不存在（404）处理避免探测；可写性是用户正在执行的操作，返回 403
// 「超出数据范围」能说清为什么没做成，比 404 更可诊断。
// 判据本身一致：看得到就管得了。
func (s *Scope) EnsureDeptWritable(id int64) error {
	if s.AllowsDept(id) {
		return nil
	}
	return errcode.New(errcode.ErrForbidden, "超出数据范围")
}

// DeptTreeIDs 返回部门及其所有子部门 ID（BFS）。
//
// 部门树是全量读一遍后在内存里展开的：sys_depts 本身只有几十行，
// 用递归 CTE 反而不如直接建邻接表直观。注意必须带 visited 保护——
// 若库里存在环（parent_id 手工改错），BFS 会无限循环把内存打满。
func DeptTreeIDs(rootID int64) ([]int64, error) {
	var all []model.Dept
	if err := database.DB().Select("id", "parent_id").Find(&all).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	children := map[int64][]int64{}
	for _, d := range all {
		children[d.ParentID] = append(children[d.ParentID], d.ID)
	}
	result := []int64{rootID}
	visited := map[int64]bool{rootID: true}
	queue := []int64{rootID}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, cid := range children[cur] {
			if visited[cid] {
				continue
			}
			visited[cid] = true
			result = append(result, cid)
			queue = append(queue, cid)
		}
	}
	return result, nil
}
