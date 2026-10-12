package user

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"nova-admin/internal/authz"
	"nova-admin/internal/model"
	"nova-admin/internal/pkg/dberr"
	"nova-admin/internal/pkg/dbquery"
	"nova-admin/internal/pkg/errcode"
	"nova-admin/internal/pkg/hash"
	"nova-admin/internal/pkg/sheet"
	"nova-admin/internal/pkg/textutil"
)

const (
	// maxImportRows 单次导入行数上限。每行都要做 bcrypt 哈希，行数越多越慢
	maxImportRows = 1000
	// DefaultImportPassword 是导入时「密码列留空」所使用的默认密码
	DefaultImportPassword = "123456"
)

// 导入模板的列顺序。务必与 buildImportTemplate() 中的列定义保持一致，
// 否则会出现「表头与数据错位」这种极难排查的问题。
const (
	colUsername = iota
	colNickname
	colPassword
	colEmail
	colPhone
	colDept
	colRoles
	colStatus
	colRemark
	importColumnCount
)

var (
	usernameRe = regexp.MustCompile(`^[A-Za-z0-9_.\-]+$`)
	emailRe    = regexp.MustCompile(`^[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}$`)
)

// ---------------- 导出 ----------------

// Export 查询待导出的用户。指定了 ids 时按选中项导出，否则按筛选条件导出。
//
// 数据范围与列表共用同一套 where 构造：导出的行集必须与页面看到的一致，
// 否则会出现「页面只有 10 条、导出是全表」这种数据外带级别的缺陷。
func (s *Service) Export(keyword string, f ExportQuery, actor *authz.Identity) ([]VO, error) {
	tx := s.scopeQuery(actor, s.db.Model(&model.User{})).Preload("Dept").Preload("Roles")

	// 「未提供 ids」与「提供了 ids 但一个都解析不出来」必须区别对待：
	// 前者是按筛选导出，后者是参数有问题。若把后者也当成前者，
	// 一次请求 ids=garbage 就会静默变成导出全表 —— 请求语义被悄悄放大。
	if strings.TrimSpace(f.IDs) != "" {
		ids := parseIDList(f.IDs)
		if len(ids) == 0 {
			return nil, errcode.New(errcode.ErrBadRequest, "选中项参数无效，请重新勾选后导出")
		}
		if len(ids) > sheet.MaxExportRows {
			return nil, errcode.Newf(errcode.ErrBadRequest,
				"单次最多导出 %d 条，当前选中 %d 条，请分批导出", sheet.MaxExportRows, len(ids))
		}
		tx = tx.Where("id IN (?)", ids)
	} else {
		tx = s.buildFilter(keyword, f.ListQuery)(tx)
	}

	// 多查一条用于判断是否超限（dbquery 内部处理，调用方只关心 exceeded）
	users, exceeded, err := dbquery.FetchWithCap[model.User](tx.Order("id ASC"), sheet.MaxExportRows)
	if err != nil {
		return nil, err
	}
	if exceeded {
		return nil, errcode.Newf(errcode.ErrBadRequest,
			"单次导出上限 %d 条，请缩小筛选范围后重试", sheet.MaxExportRows)
	}

	items := make([]VO, 0, len(users))
	for i := range users {
		items = append(items, toVO(&users[i]))
	}
	return items, nil
}

// BuildExportFile 把用户列表渲染成 xlsx 字节流。
func (s *Service) BuildExportFile(items []VO) ([]byte, error) {
	tbl := sheet.NewTable[VO]("用户数据").
		Col("ID", 8, func(v VO) string { return strconv.FormatInt(v.ID, 10) }).
		Col("用户名", 16, func(v VO) string { return v.Username }).
		Col("昵称", 14, func(v VO) string { return v.Nickname }).
		Col("邮箱", 24, func(v VO) string { return v.Email }).
		Col("手机号", 16, func(v VO) string { return v.Phone }).
		Col("部门", 16, func(v VO) string { return v.DeptName }).
		Col("角色", 20, func(v VO) string { return strings.Join(v.RoleNames, "、") }).
		Col("状态", 9, func(v VO) string { return StatusText(v.Status) }).
		Col("备注", 24, func(v VO) string { return v.Remark }).
		Col("最后登录时间", 20, func(v VO) string { return formatTimePtr(v.LastLoginAt) }).
		Col("最后登录IP", 16, func(v VO) string { return v.LastLoginIP }).
		Col("创建时间", 20, func(v VO) string { return textutil.FormatTime(v.CreatedAt) }).
		Rows(items)
	return tbl.Build()
}

// ---------------- 导入 ----------------

// buildImportTemplate 生成导入模板。
//
// 模板的数据区刻意留空：如果放示例行，用户很可能不删除就直接上传，
// 把示例数据一并导进系统。格式示例统一写在「填写说明」工作表中。
func buildImportTemplate() ([]byte, error) {
	tbl := sheet.NewTable[[]string]("用户导入").
		Col("用户名", 18, func(r []string) string { return sheet.Cell(r, colUsername) }).
		Col("昵称", 14, func(r []string) string { return sheet.Cell(r, colNickname) }).
		Col("密码", 14, func(r []string) string { return sheet.Cell(r, colPassword) }).
		Col("邮箱", 24, func(r []string) string { return sheet.Cell(r, colEmail) }).
		Col("手机号", 16, func(r []string) string { return sheet.Cell(r, colPhone) }).
		Col("部门", 16, func(r []string) string { return sheet.Cell(r, colDept) }).
		Col("角色", 20, func(r []string) string { return sheet.Cell(r, colRoles) }).
		Col("状态", 10, func(r []string) string { return sheet.Cell(r, colStatus) }).
		Col("备注", 24, func(r []string) string { return sheet.Cell(r, colRemark) }).
		Notes(
			"用户导入模板 · 填写说明",
			"",
			"【基本规则】",
			"1. 请在「用户导入」工作表中从第 2 行开始填写数据。",
			"2. 第一行是表头，请勿删除或调整列的顺序，否则导入会被拒绝。",
			"3. 带 * 的列为必填项。",
			"4. 填写完全部数据后，回到系统点击「导入用户」上传本文件。系统会先校验并展示逐行结果，",
			"   确认无误后再执行导入，不会直接写库。",
			"",
			"【各列填写规则】",
			"用户名 *：3-64 个字符，只能包含字母、数字、下划线、点、横线；不能与系统中已有用户重复。",
			"昵称 *　：不超过 64 个字符。",
			"密码　 ：留空则使用默认密码 "+DefaultImportPassword+"，导入后请提醒用户尽快修改。如填写需 6-64 位。",
			"邮箱　 ：选填，填写时必须是合法邮箱格式，例如 zhangsan@example.com。",
			"手机号：选填，不超过 32 个字符。",
			"部门　 ：选填，需与「系统管理 - 部门管理」中的部门名称完全一致，否则该行会被拒绝。",
			"角色　 ：选填，多个角色用「、」或「,」分隔，例如：系统管理员、只读访客。",
			"状态　 ：选填，可填「启用」或「禁用」，留空默认启用。",
			"备注　 ：选填，不超过 255 个字符。",
			"",
			"【填写示例（请勿直接复制到数据区，按实际情况修改）】",
			"zhangsan | 张三 | (留空) | zhangsan@example.com | 13800138000 | 技术部 | 只读访客 | 启用 | 新入职",
		)
	return tbl.Build()
}

// Import 解析并导入用户。dryRun 为 true 时只校验不写库。
func (s *Service) Import(data []byte, dryRun bool, actor *authz.Identity) (*ImportResult, error) {
	rows, err := sheet.Parse(data)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errcode.New(errcode.ErrBadRequest, "文件内容为空，请先下载模板后填写")
	}
	if err := validateImportHeader(rows[0]); err != nil {
		return nil, err
	}

	// 剔除空白行，并保留其在 Excel 中的真实行号，便于用户定位
	type line struct {
		no    int
		cells []string
	}
	lines := make([]line, 0, len(rows)-1)
	for i, raw := range rows[1:] {
		if sheet.IsBlankRow(raw) {
			continue
		}
		lines = append(lines, line{no: i + 2, cells: raw})
	}
	if len(lines) == 0 {
		return nil, errcode.New(errcode.ErrBadRequest, "文件中没有可导入的数据行，请在表头下方填写数据")
	}
	if len(lines) > maxImportRows {
		return nil, errcode.Newf(errcode.ErrBadRequest,
			"单次导入上限 %d 行，当前文件有 %d 行，请拆分后分批导入", maxImportRows, len(lines))
	}

	// 一次性加载参照数据，避免逐行查库
	ref, err := s.loadImportRefs()
	if err != nil {
		return nil, err
	}
	// 导入同样受数据范围约束：否则「本部门」管理员可以用一份 Excel
	// 把用户批量建到别的部门去，绕过单条新增上的校验。nil 表示不受限。
	allowedDepts := s.scopeDeptIDs(actor)

	result := &ImportResult{Total: len(lines), Rows: make([]ImportRow, 0, len(lines))}
	candidates := make([]importCandidate, 0, len(lines))

	seen := make(map[string]bool, len(lines))
	for _, ln := range lines {
		detail := ImportRow{
			Row:      ln.no,
			Username: sheet.Cell(ln.cells, colUsername),
			Nickname: sheet.Cell(ln.cells, colNickname),
			Errors:   make([]string, 0, 2),
		}
		cand, errs := s.validateImportLine(ln.cells, ref, seen, allowedDepts)
		detail.Errors = append(detail.Errors, errs...)

		if len(detail.Errors) > 0 {
			result.Invalid++
		} else {
			result.Valid++
			candidates = append(candidates, cand)
		}
		result.Rows = append(result.Rows, detail)
	}

	if !dryRun && len(candidates) > 0 {
		// 授予校验必须在这一步做：逐行校验阶段只判「角色是否存在」，
		// 那里无法汇总（同一个越权角色可能出现在任意一行），也没必要为每行都查一遍权限。
		if err := s.ensureImportGrantable(actor, candidates); err != nil {
			return nil, err
		}
		created, err := s.bulkCreateUsers(candidates)
		if err != nil {
			return nil, err
		}
		result.Created = created
	}
	return result, nil
}

// ensureImportGrantable 校验本次导入涉及的角色是否都在调用者的可授予范围内。
//
// 为什么必须有这一步：导入的写库路径（bulkCreateUsers）直接往 sys_user_roles 插关联行，
// 完全绕过 replaceRoles 里的 authz.EnsureGrantable。若不在此处补校验，
// 任何持有 system:user:import 的非超管都能在 Excel 的「角色」列填上「超级管理员」，
// 批量造出超管账号 —— 相当于给提权开了一条与单条新增并行的旁路。
func (s *Service) ensureImportGrantable(actor *authz.Identity, cands []importCandidate) error {
	if actor == nil || actor.IsSuper || len(cands) == 0 {
		return nil
	}
	// 先把候选行里用到的角色 ID 去重，避免同一个角色被反复查询
	used := make(map[int64]bool, 4)
	ids := make([]int64, 0, 4)
	for _, c := range cands {
		for _, id := range c.roles {
			if !used[id] {
				used[id] = true
				ids = append(ids, id)
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	roles := make([]model.Role, 0, len(ids))
	// 不按 status 过滤：禁用角色同样要参与判定，否则「传一个禁用的高权限角色」
	// 会因为查不出来而被当成没传，等于留了一条绕过通道。
	if err := s.db.Select("id", "name", "code", "data_scope").
		Where("id IN ?", ids).
		Find(&roles).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	// 与单条新增走同一套判定，避免两条路径的规则漂移
	return authz.EnsureGrantable(actor, roles)
}

// importCandidate 是一行「校验通过、待写入」的数据。
type importCandidate struct {
	// user 只承载字段值，其 Password 由 plainPassword 在写入前统一哈希填充
	user          model.User
	plainPassword string
	roles         []int64
}

// validateImportLine 校验单行，返回可直接入库的候选数据。
// allowedDepts 为 nil 表示不受数据范围限制，否则只有集合内的部门可被使用。
func (s *Service) validateImportLine(cells []string, ref *importRefs, seen map[string]bool, allowedDepts map[int64]bool) (importCandidate, []string) {
	errs := make([]string, 0, 2)

	username := sheet.Cell(cells, colUsername)
	nickname := sheet.Cell(cells, colNickname)
	password := sheet.Cell(cells, colPassword)
	email := sheet.Cell(cells, colEmail)
	phone := sheet.Cell(cells, colPhone)
	deptName := sheet.Cell(cells, colDept)
	roleNames := sheet.Cell(cells, colRoles)
	statusRaw := sheet.Cell(cells, colStatus)
	remark := sheet.Cell(cells, colRemark)

	// ---- 用户名 ----
	// 这里只判断用户名本身是否可用；是否记入 seen（用于「文件内重复」判定）
	// 必须等整行校验结束后再决定，理由见文件末尾。
	switch {
	case username == "":
		errs = append(errs, "用户名不能为空")
	case len([]rune(username)) < 3 || len([]rune(username)) > 64:
		errs = append(errs, "用户名长度需为 3-64 个字符")
	case !usernameRe.MatchString(username):
		errs = append(errs, "用户名只能包含字母、数字、下划线、点和横线")
	case ref.usernames[username]:
		errs = append(errs, "用户名已被系统中其他用户使用")
	case seen[username]:
		errs = append(errs, "与文件中的其他行用户名重复")
	}

	// ---- 昵称 ----
	if nickname == "" {
		errs = append(errs, "昵称不能为空")
	} else if len([]rune(nickname)) > 64 {
		errs = append(errs, "昵称不能超过 64 个字符")
	}

	// ---- 密码 ----
	if password != "" && (len([]rune(password)) < 6 || len([]rune(password)) > 64) {
		errs = append(errs, "密码长度需为 6-64 个字符")
	}

	// ---- 邮箱 ----
	if email != "" {
		if len(email) > 128 {
			errs = append(errs, "邮箱不能超过 128 个字符")
		} else if !emailRe.MatchString(email) {
			errs = append(errs, "邮箱格式不正确")
		}
	}

	// ---- 手机号 ----
	if len([]rune(phone)) > 32 {
		errs = append(errs, "手机号不能超过 32 个字符")
	}

	// ---- 备注 ----
	if len([]rune(remark)) > 255 {
		errs = append(errs, "备注不能超过 255 个字符")
	}

	// ---- 部门 ----
	var deptID *int64
	if deptName != "" {
		if id, ok := ref.depts[deptName]; ok {
			deptID = &id
		} else {
			errs = append(errs, "部门「"+deptName+"」不存在")
		}
	}
	if allowedDepts != nil {
		if deptID == nil {
			errs = append(errs, "请填写你数据范围内的部门")
		} else if !allowedDepts[*deptID] {
			errs = append(errs, "部门「"+deptName+"」超出你的数据范围")
		}
	}

	// ---- 角色 ----
	// 必须去重：同一个角色用「、」「,」「；」混写两遍是很常见的笔误，
	// 而 sys_user_roles 是 (user_id, role_id) 复合主键 —— 重复项会让
	// 整批 INSERT 触发主键冲突，结果是「预览显示全部通过、导入却整单失败」。
	roleIDs := make([]int64, 0, 2)
	seenRoleName := make(map[string]bool, 2)
	for _, name := range splitNames(roleNames) {
		if seenRoleName[name] {
			continue
		}
		seenRoleName[name] = true
		id, ok := ref.roles[name]
		if !ok {
			errs = append(errs, "角色「"+name+"」不存在")
			continue
		}
		roleIDs = append(roleIDs, id)
	}

	// ---- 状态 ----
	status, statusOK := parseImportStatus(statusRaw)
	if !statusOK {
		errs = append(errs, "状态只能填写「启用」或「禁用」")
	}

	// 只有整行都通过校验（即真的会入库）时才把用户名记入 seen。
	//
	// 旧写法在「用户名本身合法」时就立刻标记，于是出现：
	// 第 3 行用户名重复但昵称为空（该行被拒绝、不会入库），
	// 第 7 行同名且完全合法，却被判成「与文件中的其他行用户名重复」而拒绝 ——
	// 用户看到两条都失败，却只被告知其中一条的真实原因。
	if len(errs) == 0 {
		seen[username] = true
	}

	return importCandidate{
		user: model.User{
			Username: username,
			Nickname: nickname,
			Email:    email,
			Phone:    phone,
			DeptID:   deptID,
			Status:   status,
			Remark:   remark,
		},
		plainPassword: password,
		roles:         roleIDs,
	}, errs
}

// bulkCreateUsers 在一个事务里批量写入用户及其角色关联。
func (s *Service) bulkCreateUsers(candidates []importCandidate) (int, error) {
	// 相同明文只计算一次哈希：导入场景下绝大多数行使用默认密码，
	// 逐行 bcrypt 会让 1000 行的导入耗时从毫秒级涨到分钟级。
	hashCache := make(map[string]string, 4)
	users := make([]model.User, 0, len(candidates))
	for _, c := range candidates {
		plain := DefaultImportPassword
		if c.plainPassword != "" {
			plain = c.plainPassword
		}
		h, ok := hashCache[plain]
		if !ok {
			var err error
			h, err = hash.Password(plain)
			if err != nil {
				return 0, errcode.Wrap(errcode.ErrInternal, err)
			}
			hashCache[plain] = h
		}
		u := c.user
		u.Password = h
		users = append(users, u)
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		// 分批写入，避免单条 INSERT 语句参数过多
		if err := tx.CreateInBatches(&users, 200).Error; err != nil {
			return err
		}
		links := make([]model.UserRole, 0, len(candidates)*2)
		for i := range users {
			for _, rid := range candidates[i].roles {
				links = append(links, model.UserRole{UserID: users[i].ID, RoleID: rid})
			}
		}
		if len(links) == 0 {
			return nil
		}
		return tx.CreateInBatches(&links, 500).Error
	})
	if err != nil {
		// 唯一键冲突一般意味着「预览与实际写库之间数据变了」（别人同时建了同名用户）。
		// 直接抛「数据库操作失败」用户完全无从下手，这里换成可操作的提示并回 409。
		if dberr.IsUniqueViolation(err) {
			return 0, errcode.New(errcode.ErrConflict,
				"导入时出现唯一性冲突，可能有同名用户被同时创建，请重新上传文件校验后再试")
		}
		return 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	return len(users), nil
}

// importRefs 汇总导入校验所需的参照数据。
type importRefs struct {
	depts     map[string]int64 // 部门名 -> ID
	roles     map[string]int64 // 角色名 -> ID
	usernames map[string]bool  // 已存在的用户名
}

func (s *Service) loadImportRefs() (*importRefs, error) {
	ref := &importRefs{
		depts:     make(map[string]int64),
		roles:     make(map[string]int64),
		usernames: make(map[string]bool),
	}

	var depts []model.Dept
	if err := s.db.Select("id", "name").Find(&depts).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	for _, d := range depts {
		ref.depts[d.Name] = d.ID
	}

	var roles []model.Role
	// 与 role.Options 保持同一口径：禁用角色不参与导入。
	// 否则会出现「页面上根本选不到这个角色，但 Excel 里填名字就能授出去」的旁路，
	// 而且授完也不生效（authz.build 只加载启用角色），用户还以为是 bug。
	if err := s.db.Select("id", "name").Where("status = ?", model.StatusEnabled).Find(&roles).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	for _, r := range roles {
		ref.roles[r.Name] = r.ID
	}

	var names []string
	if err := s.db.Model(&model.User{}).Pluck("username", &names).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	for _, n := range names {
		ref.usernames[n] = true
	}
	return ref, nil
}

// validateImportHeader 校验表头，防止用户误传了别的表格。
func validateImportHeader(header []string) error {
	// 与列定义同源，新增列时只需改 importColumns 一处
	for _, c := range importColumns() {
		if got := sheet.Cell(header, c.idx); got != c.name {
			return errcode.Newf(errcode.ErrBadRequest,
				"第 %d 列的表头应为「%s」，实际为「%s」，请使用系统提供的模板填写",
				c.idx+1, c.name, got)
		}
	}
	return nil
}

// importColumn 描述导入表的一列。
type importColumn struct {
	idx  int
	name string
}

func importColumns() []importColumn {
	return []importColumn{
		{colUsername, "用户名"},
		{colNickname, "昵称"},
		{colPassword, "密码"},
		{colEmail, "邮箱"},
		{colPhone, "手机号"},
		{colDept, "部门"},
		{colRoles, "角色"},
		{colStatus, "状态"},
		{colRemark, "备注"},
	}
}

// parseImportStatus 解析状态列，空值视为启用。
func parseImportStatus(s string) (int16, bool) {
	switch strings.ToLower(s) {
	case "", "启用", "1", "true", "yes", "y", "enabled":
		return model.StatusEnabled, true
	case "禁用", "0", "false", "no", "n", "disabled":
		return model.StatusDisabled, true
	default:
		return model.StatusEnabled, false
	}
}

// splitNames 按中英文顿号、逗号、斜杠、分号切分多值单元格。
func splitNames(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	fields := strings.FieldsFunc(s, func(r rune) bool {
		switch r {
		case ',', '，', '、', '/', ';', '；':
			return true
		default:
			return false
		}
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if v := strings.TrimSpace(f); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// parseIDList 解析逗号分隔的 ID 串，非法项直接忽略。
func parseIDList(s string) []int64 {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]int64, 0, len(parts))
	for _, p := range parts {
		id, err := strconv.ParseInt(strings.TrimSpace(p), 10, 64)
		if err != nil || id <= 0 {
			continue
		}
		out = append(out, id)
	}
	return out
}

// StatusText 返回状态的中文文案，供导出与其它展示场景复用。
func StatusText(status int16) string {
	if status == model.StatusEnabled {
		return "启用"
	}
	return "禁用"
}

func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return textutil.FormatTime(*t)
}
