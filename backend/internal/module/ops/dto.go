package ops

import "time"

// RunBackupReq 执行备份请求。
type RunBackupReq struct {
	// SkipPrune 为 true 时不做轮转（升级前留档用）。
	SkipPrune bool   `json:"skipPrune" label:"不做轮转，单独留档"`
	Note      string `json:"note" label:"备注"`
}

// DeleteBackupReq 删除备份请求。用文件名而不是路径：
// 路径一旦由客户端给出，就必然要防 `../` 穿越，而文件名只需在目录内比对。
type DeleteBackupReq struct {
	File string `json:"file" label:"备份文件名" binding:"required"`
}

// DownloadBackupReq 下载备份请求。
type DownloadBackupReq struct {
	File string `json:"file" form:"file" label:"备份文件名" binding:"required"`
}

// RestoreReq 恢复演练请求。
type RestoreReq struct {
	File string `json:"file" label:"备份文件名" binding:"required"`
	// Target 目标库名。留空自动生成 <生产库>_restore_<时间戳>；
	// **不允许**指向生产库本身（命令行要 --force，接口干脆不给这个能力）。
	Target string `json:"target" label:"目标库名"`
}

// CleanupApplyReq 真删请求。
type CleanupApplyReq struct {
	// Confirm 必须正好等于「确认清理」。清理不可逆，多打四个字换一道闸门。
	Confirm string `json:"confirm" label:"确认口令" binding:"required"`

	OpLogDays       int `json:"opLogDays" label:"操作日志保留天数"`
	LoginLogDays    int `json:"loginLogDays" label:"登录日志保留天数"`
	IdempotencyDays int `json:"idempotencyDays" label:"幂等键保留天数"`
}

// UploadPackageReq 上传升级包。
type UploadPackageReq struct {
	File string `json:"-"` // multipart 文件名，只用于报错信息
	Data []byte `json:"-"`
}

// UpgradeBackupReq 升级前留档请求。
type UpgradeBackupReq struct {
	PlanID string `json:"planId" label:"升级计划标识" binding:"required"`
}

// ApplyUpgradeReq 执行升级请求。
type ApplyUpgradeReq struct {
	PlanID string `json:"planId" label:"升级计划标识" binding:"required"`
	// Confirm 必须正好等于「确认升级」。
	Confirm string `json:"confirm" label:"确认口令" binding:"required"`
}

// BackupItem 是一个备份文件的展示形态。
type BackupItem struct {
	Name          string    `json:"name"`
	Path          string    `json:"path"`
	Size          int64     `json:"size"`
	Time          time.Time `json:"time"`
	SHA256        string    `json:"sha256"`
	Database      string    `json:"database"`
	ServerVersion string    `json:"serverVersion"`
	Tables        int64     `json:"tables"`
	HasMeta       bool      `json:"hasMeta"`
}

// CleanupView 是清理结果的展示形态。
type CleanupView struct {
	DryRun   bool          `json:"dryRun"`
	Steps    []CleanupStep `json:"steps"`
	Vacuumed []string      `json:"vacuumed"`
	Blocked  string        `json:"blocked"` // 非空表示被拦下（确认口令不对等）
}

// CleanupStep 与 ops.CleanupStep 同形，单独定义是为了让 JSON tag 稳定，
// 不随框架包的结构调整而变。
type CleanupStep struct {
	Name    string    `json:"name"`
	Table   string    `json:"table"`
	Cutoff  time.Time `json:"cutoff"`
	Rows    int64     `json:"rows"`
	Batches int       `json:"batches"`
	Note    string    `json:"note"`
}

// PreflightItem 是升级前预检的一项。
type PreflightItem struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	Pass  bool   `json:"pass"`
	Hint  string `json:"hint"`
}

// UpgradeStatus 是运维中心「系统升级」页签的初始数据。
type UpgradeStatus struct {
	Release        string          `json:"release"`
	SchemaDB       int32           `json:"schemaDb"`
	SchemaBinary   int32           `json:"schemaBinary"`
	SchemaKnown    bool            `json:"schemaKnown"`
	SchemaNote     string          `json:"schemaNote"`
	LatestBackup   string          `json:"latestBackup"`
	LatestBackupAt time.Time       `json:"latestBackupAt"`
	Database       string          `json:"database"`
	ServerVersion  string          `json:"serverVersion"`
	SizeBytes      int64           `json:"sizeBytes"`
	Preflight      []PreflightItem `json:"preflight"`
	Records        []UpgradeRecord `json:"records"`
	Warnings       []string        `json:"warnings"`
	RestartHint    string          `json:"restartHint"`
}

// UpgradeRecord 与 model.UpgradeRecord 同形，附带解析后的动作清单。
type UpgradeRecord struct {
	ID            uint            `json:"id"`
	PlanID        string          `json:"planId"`
	FromVersion   string          `json:"fromVersion"`
	ToVersion     string          `json:"toVersion"`
	PackageSHA256 string          `json:"packageSha256"`
	PackageSize   int64           `json:"packageSize"`
	BackupPath    string          `json:"backupPath"`
	Status        string          `json:"status"`
	Actions       []UpgradeAction `json:"actions"`
	Message       string          `json:"message"`
	CreatedAt     time.Time       `json:"createdAt"`
	AppliedAt     *time.Time      `json:"appliedAt"`
	FinishedAt    *time.Time      `json:"finishedAt"`
}

// UpgradeAction 与 ops.UpgradeAction 同形。
type UpgradeAction struct {
	Key    string `json:"key"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Done   bool   `json:"done"`
	Error  string `json:"error"`
}
