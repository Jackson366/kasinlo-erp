package com.kasinlo.erp.persistence.entity;

import com.baomidou.mybatisplus.annotation.TableField;
import com.baomidou.mybatisplus.annotation.TableName;
import java.math.BigDecimal;
import java.time.LocalDate;
import java.time.OffsetDateTime;

/** Persistence mapping for inv_attachments. */
@TableName("inv_attachments")
public class InvAttachments extends SoftDeleteEntity {
    @TableField("\"biz_type\"")
    private String bizType;

    @TableField("\"biz_id\"")
    private Long bizId;

    @TableField("\"file_name\"")
    private String fileName;

    @TableField("\"stored_path\"")
    private String storedPath;

    @TableField("\"size\"")
    private Long size;

    @TableField("\"mime\"")
    private String mime;

    @TableField("\"uploaded_by\"")
    private Long uploadedBy;

    @TableField("\"uploader\"")
    private String uploader;

    public String getBizType() { return bizType; }
    public void setBizType(String bizType) { this.bizType = bizType; }

    public Long getBizId() { return bizId; }
    public void setBizId(Long bizId) { this.bizId = bizId; }

    public String getFileName() { return fileName; }
    public void setFileName(String fileName) { this.fileName = fileName; }

    public String getStoredPath() { return storedPath; }
    public void setStoredPath(String storedPath) { this.storedPath = storedPath; }

    public Long getSize() { return size; }
    public void setSize(Long size) { this.size = size; }

    public String getMime() { return mime; }
    public void setMime(String mime) { this.mime = mime; }

    public Long getUploadedBy() { return uploadedBy; }
    public void setUploadedBy(Long uploadedBy) { this.uploadedBy = uploadedBy; }

    public String getUploader() { return uploader; }
    public void setUploader(String uploader) { this.uploader = uploader; }

}
