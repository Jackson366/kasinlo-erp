package inv

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"nova-erp/internal/authz"
	"nova-erp/internal/model"
	"nova-erp/internal/pkg/errcode"
)

// 业务附件（采购单 / 销售单 / 合同的图片与文件）。
//
// 三条设计线：
//
//  1. **文件不进库**。扫描件动辄几百 KB，塞进 bytea 会让「每天一份」的数据库备份
//     涨到几百 MB；库里只留元数据与相对路径。
//  2. **磁盘上不用原始文件名**。用随机名 + 扩展名：原始名可能重名、可能含中文、
//     可能含 `../`，而下载时的文件名从库里读（客户端拿到的仍是原始名）。
//  3. **权限跟着单据走**。能看见这张单据（含数据范围）才能看它的附件 ——
//     单独设一套 attachment 权限码，早晚会出现「能改单、看不到附件」的怪组合。

// maxAttachmentBytes 单个附件上限 20 MB。
//
// 手机拍的合同照片、扫描 PDF 通常 1~5 MB，20 MB 足够；
// 再大就该走网盘，而不是塞进一个要每天备份的目录。
const maxAttachmentBytes = 20 << 20

// maxMultipartSlack 给 multipart 边界与其它表单字段留的余量。
const maxMultipartSlack = 1 << 20

// attachmentExts 允许的扩展名白名单（小写、不含点）。
//
// 用白名单而不是黑名单：业务附件只可能是图片、Office 文档、PDF 与压缩包。
// exe / bat / js / htm 这类东西对业务没有任何用处，却可能在某个时刻被当成
// 「附件」双击打开 —— 拒绝在入口处，比事后审计便宜得多。
var attachmentExts = map[string]struct{}{
	// 图片
	"jpg": {}, "jpeg": {}, "png": {}, "gif": {}, "webp": {}, "bmp": {},
	// 文档
	"pdf": {}, "doc": {}, "docx": {}, "xls": {}, "xlsx": {}, "ppt": {}, "pptx": {}, "txt": {}, "csv": {},
	// 压缩包
	"zip": {}, "rar": {}, "7z": {},
}

// attachmentMime 按扩展名推 MIME，**不信客户端声明的 Content-Type**。
//
// 客户端说自己是 image/png、实际是 html，浏览器如果按 inline 渲染就是一次存储型 XSS。
// 服务端按扩展名给类型，再配上 X-Content-Type-Options: nosniff，浏览器不会去猜。
func attachmentMime(ext string) string {
	switch strings.TrimPrefix(ext, ".") {
	case "jpg", "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	case "bmp":
		return "image/bmp"
	case "pdf":
		return "application/pdf"
	case "txt":
		return "text/plain; charset=utf-8"
	case "csv":
		return "text/csv; charset=utf-8"
	case "doc":
		return "application/msword"
	case "docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case "xls":
		return "application/vnd.ms-excel"
	case "xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case "ppt":
		return "application/vnd.ms-powerpoint"
	case "pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case "zip":
		return "application/zip"
	case "rar":
		return "application/vnd.rar"
	case "7z":
		return "application/x-7z-compressed"
	default:
		return "application/octet-stream"
	}
}

// isImageExt 判断是否图片：前端据此决定「直接预览」还是「下载」。
func isImageExt(ext string) bool {
	switch strings.TrimPrefix(ext, ".") {
	case "jpg", "jpeg", "png", "gif", "webp", "bmp":
		return true
	}
	return false
}

// AttachmentVO 附件出参。
type AttachmentVO struct {
	ID        int64     `json:"id"`
	BizType   string    `json:"bizType"`
	BizID     int64     `json:"bizId"`
	FileName  string    `json:"fileName"`
	Ext       string    `json:"ext"`
	Size      int64     `json:"size"`
	Mime      string    `json:"mime"`
	IsImage   bool      `json:"isImage"`
	Uploader  string    `json:"uploader"`
	CreatedAt time.Time `json:"createdAt"`
}

func toAttachmentVO(a model.InvAttachment) AttachmentVO {
	ext := strings.ToLower(filepath.Ext(a.FileName))
	return AttachmentVO{
		ID: a.ID, BizType: a.BizType, BizID: a.BizID, FileName: a.FileName,
		Ext: ext, Size: a.Size, Mime: a.Mime, IsImage: isImageExt(ext),
		Uploader: a.Uploader, CreatedAt: a.CreatedAt,
	}
}

// attachmentRoot 返回附件存储根目录。
//
// config 已在启动时把相对路径解析成「exe 所在目录」下的绝对路径；
// 兜底分支只在测试（未加载配置）时走到。
func (s *Service) attachmentRoot() string {
	if s.attachDir != "" {
		return s.attachDir
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "data", "attachments")
	}
	return filepath.Join("data", "attachments")
}

// ensureAttachmentBiz 校验附件所属业务对象存在且在数据范围内。
//
// 「不存在」与「越权」都返回 404：用 403 会把「这张单存在、只是不归你」
// 泄漏给调用方，等于给了个探测别人单据的接口。
func (s *Service) ensureAttachmentBiz(bizType string, bizID int64, actor *authz.Identity) error {
	if bizID <= 0 {
		return errcode.New(errcode.ErrBadRequest, "缺少业务对象 ID")
	}
	var creator int64
	switch bizType {
	case model.InvAttachPurchase, model.InvAttachSales:
		var row struct{ CreatedBy int64 }
		err := s.db.Model(&model.InvOrder{}).
			Select("created_by").
			Where("id = ? AND biz_type = ?", bizID, bizType).
			Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.New(errcode.ErrNotFound, "单据不存在")
		}
		if err != nil {
			return errcode.Wrap(errcode.ErrDatabase, err)
		}
		creator = row.CreatedBy
	case model.InvAttachContract:
		var row struct{ CreatedBy int64 }
		err := s.db.Model(&model.InvContract{}).
			Select("created_by").
			Where("id = ?", bizID).
			Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.New(errcode.ErrNotFound, "合同不存在")
		}
		if err != nil {
			return errcode.Wrap(errcode.ErrDatabase, err)
		}
		creator = row.CreatedBy
	default:
		return errcode.New(errcode.ErrBadRequest, "附件所属业务类型非法")
	}
	return s.ensureCreatorVisible(creator, actor)
}

// randomStoredName 生成磁盘文件名：16 字节随机数的十六进制。
func randomStoredName() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// UploadAttachment 接收一个上传文件并落库。
func (s *Service) UploadAttachment(bizType string, bizID int64, fh *multipart.FileHeader, actor *authz.Identity) (*AttachmentVO, error) {
	if err := s.ensureAttachmentBiz(bizType, bizID, actor); err != nil {
		return nil, err
	}
	// 只取基名：客户端可能传 D:\合同\扫描件.pdf 或 ../../x.pdf，
	// 真正的磁盘路径由我们生成，原始名只用于显示与下载。
	name := strings.TrimSpace(filepath.Base(fh.Filename))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return nil, errcode.New(errcode.ErrBadRequest, "文件名不能为空")
	}
	// 数据库字段 size:255，超长的名字截断前 200 字节再拼扩展名
	if len([]rune(name)) > 120 {
		name = string([]rune(name)[:120])
	}
	ext := strings.ToLower(filepath.Ext(name))
	if _, ok := attachmentExts[strings.TrimPrefix(ext, ".")]; !ok {
		return nil, errcode.Newf(errcode.ErrBadRequest,
			"不支持的文件类型「%s」，可上传图片、PDF、Office 文档与压缩包", ext)
	}
	if fh.Size > maxAttachmentBytes {
		return nil, errcode.Newf(errcode.ErrBadRequest, "附件超过 %d MB", maxAttachmentBytes>>20)
	}
	if fh.Size <= 0 {
		return nil, errcode.New(errcode.ErrBadRequest, "附件是空文件")
	}

	rel, err := s.storeAttachment(bizType, bizID, ext, fh)
	if err != nil {
		return nil, err
	}

	uploader := ""
	if actor != nil {
		uploader = actor.Nickname
		if uploader == "" {
			uploader = actor.Username
		}
	}
	att := model.InvAttachment{
		BizType: bizType, BizID: bizID, FileName: name,
		StoredPath: rel, Size: fh.Size, Mime: attachmentMime(ext),
		UploadedBy: actorID(actor), Uploader: uploader,
	}
	if err := s.db.Create(&att).Error; err != nil {
		// 落库失败必须把刚落盘的文件删掉，否则附件目录会攒下一堆
		// 「磁盘上有、库里查不到」的孤儿文件，谁也不敢清理。
		_ = os.Remove(filepath.Join(s.attachmentRoot(), rel))
		return nil, saveErr(err)
	}
	vo := toAttachmentVO(att)
	return &vo, nil
}

// storeAttachment 把上传内容写到磁盘，返回**相对**存储根的路径。
//
// 目录按 bizType/bizID/年月 分级：同一张单据的附件挨在一起方便人工排查，
// 按月再分一层是为了避免单目录下堆到几万个文件（Windows 上是真的会慢）。
func (s *Service) storeAttachment(bizType string, bizID int64, ext string, fh *multipart.FileHeader) (string, error) {
	stored, err := randomStoredName()
	if err != nil {
		return "", errcode.Wrap(errcode.ErrInternal, err)
	}
	rel := filepath.Join(bizType, strconv.FormatInt(bizID, 10), time.Now().Format("200601"), stored+ext)
	abs := filepath.Join(s.attachmentRoot(), rel)

	if err := os.MkdirAll(filepath.Dir(abs), 0o750); err != nil {
		return "", errcode.Newf(errcode.ErrInternal, "创建附件目录失败：%v", err)
	}
	src, err := fh.Open()
	if err != nil {
		return "", errcode.Newf(errcode.ErrBadRequest, "读取上传文件失败：%v", err)
	}
	defer func() { _ = src.Close() }()

	dst, err := os.OpenFile(abs, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return "", errcode.Newf(errcode.ErrInternal, "写入附件失败：%v", err)
	}
	// 用 LimitReader 再兜一次大小：fh.Size 是客户端声明的，
	// 一个声明 1 MB、实际发送 1 GB 的请求不能真的写满磁盘。
	written, copyErr := io.Copy(dst, io.LimitReader(src, maxAttachmentBytes+1))
	closeErr := dst.Close()
	if copyErr != nil {
		_ = os.Remove(abs)
		return "", errcode.Newf(errcode.ErrInternal, "写入附件失败：%v", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(abs)
		return "", errcode.Newf(errcode.ErrInternal, "保存附件失败：%v", closeErr)
	}
	if written > maxAttachmentBytes {
		_ = os.Remove(abs)
		return "", errcode.Newf(errcode.ErrBadRequest, "附件超过 %d MB", maxAttachmentBytes>>20)
	}
	return rel, nil
}

// ListAttachments 列出某个业务对象的附件（新上传的在前面）。
func (s *Service) ListAttachments(bizType string, bizID int64, actor *authz.Identity) ([]AttachmentVO, error) {
	if err := s.ensureAttachmentBiz(bizType, bizID, actor); err != nil {
		return nil, err
	}
	var rows []model.InvAttachment
	if err := s.db.Where("biz_type = ? AND biz_id = ?", bizType, bizID).
		Order("id DESC").Find(&rows).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	out := make([]AttachmentVO, 0, len(rows))
	for _, r := range rows {
		out = append(out, toAttachmentVO(r))
	}
	return out, nil
}

// AttachmentCounts 批量统计附件数量，供列表页在按钮上标记「有几张附件」。
//
// 一次查完再分组，避免列表 20 行发 20 个请求（典型 N+1）。
// 这里不重复校验数据范围：ids 来自已经按范围过滤过的列表。
func (s *Service) AttachmentCounts(bizType string, ids []int64) (map[int64]int, error) {
	out := map[int64]int{}
	if len(ids) == 0 {
		return out, nil
	}
	type row struct {
		BizID int64
		Cnt   int
	}
	var rows []row
	if err := s.db.Model(&model.InvAttachment{}).
		Select("biz_id, COUNT(*) AS cnt").
		Where("biz_type = ? AND biz_id IN ?", bizType, ids).
		Group("biz_id").Scan(&rows).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	for _, r := range rows {
		out[r.BizID] = r.Cnt
	}
	return out, nil
}

// AttachmentFile 取附件记录与磁盘绝对路径，供下载 / 预览。
func (s *Service) AttachmentFile(id int64, actor *authz.Identity) (*model.InvAttachment, string, error) {
	var att model.InvAttachment
	if err := s.db.Where("id = ?", id).Take(&att).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", errcode.New(errcode.ErrNotFound, "附件不存在")
		}
		return nil, "", errcode.Wrap(errcode.ErrDatabase, err)
	}
	if err := s.ensureAttachmentBiz(att.BizType, att.BizID, actor); err != nil {
		return nil, "", err
	}
	root := filepath.Clean(s.attachmentRoot())
	abs := filepath.Clean(filepath.Join(root, att.StoredPath))
	// 纵深防御：StoredPath 来自库，正常不会越界；万一被写入脏数据，
	// 这里拦住「把任意系统文件当附件下载」的可能。
	if !strings.HasPrefix(abs, root+string(filepath.Separator)) {
		return nil, "", errcode.New(errcode.ErrNotFound, "附件路径非法")
	}
	if _, err := os.Stat(abs); err != nil {
		return nil, "", errcode.New(errcode.ErrNotFound, "附件文件已丢失（记录在、磁盘上找不到），请联系管理员检查附件目录")
	}
	return &att, abs, nil
}

// DeleteAttachment 删除附件记录。
//
// 只软删记录，**不删磁盘文件**：误删要能找回（把 deleted_at 置空即可），
// 而附件目录本身有备份。代价是磁盘不会立刻释放 —— 换来的是「删了还能救」。
func (s *Service) DeleteAttachment(id int64, actor *authz.Identity) error {
	var att model.InvAttachment
	if err := s.db.Where("id = ?", id).Take(&att).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.New(errcode.ErrNotFound, "附件不存在")
		}
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if err := s.ensureAttachmentBiz(att.BizType, att.BizID, actor); err != nil {
		return err
	}
	if err := s.db.Delete(&model.InvAttachment{}, id).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	return nil
}

// actorID 取身份 ID，nil 安全。
func actorID(actor *authz.Identity) int64 {
	if actor == nil {
		return 0
	}
	return actor.UserID
}
