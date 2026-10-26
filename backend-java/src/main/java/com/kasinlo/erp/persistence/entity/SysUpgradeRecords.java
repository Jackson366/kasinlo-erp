package com.kasinlo.erp.persistence.entity;

import com.baomidou.mybatisplus.annotation.TableField;
import com.baomidou.mybatisplus.annotation.TableName;
import java.math.BigDecimal;
import java.time.LocalDate;
import java.time.OffsetDateTime;

/** Persistence mapping for sys_upgrade_records. */
@TableName("sys_upgrade_records")
public class SysUpgradeRecords extends IdentifiedEntity {
    @TableField("\"plan_id\"")
    private String planId;

    @TableField("\"from_version\"")
    private String fromVersion;

    @TableField("\"to_version\"")
    private String toVersion;

    @TableField("\"package_sha256\"")
    private String packageSha256;

    @TableField("\"package_size\"")
    private Long packageSize;

    @TableField("\"backup_path\"")
    private String backupPath;

    @TableField("\"status\"")
    private String status;

    @TableField("\"plan\"")
    private String plan;

    @TableField("\"message\"")
    private String message;

    @TableField("\"created_at\"")
    private OffsetDateTime createdAt;

    @TableField("\"applied_at\"")
    private OffsetDateTime appliedAt;

    @TableField("\"finished_at\"")
    private OffsetDateTime finishedAt;

    public String getPlanId() {
        return planId;
    }

    public void setPlanId(String planId) {
        this.planId = planId;
    }

    public String getFromVersion() {
        return fromVersion;
    }

    public void setFromVersion(String fromVersion) {
        this.fromVersion = fromVersion;
    }

    public String getToVersion() {
        return toVersion;
    }

    public void setToVersion(String toVersion) {
        this.toVersion = toVersion;
    }

    public String getPackageSha256() {
        return packageSha256;
    }

    public void setPackageSha256(String packageSha256) {
        this.packageSha256 = packageSha256;
    }

    public Long getPackageSize() {
        return packageSize;
    }

    public void setPackageSize(Long packageSize) {
        this.packageSize = packageSize;
    }

    public String getBackupPath() {
        return backupPath;
    }

    public void setBackupPath(String backupPath) {
        this.backupPath = backupPath;
    }

    public String getStatus() {
        return status;
    }

    public void setStatus(String status) {
        this.status = status;
    }

    public String getPlan() {
        return plan;
    }

    public void setPlan(String plan) {
        this.plan = plan;
    }

    public String getMessage() {
        return message;
    }

    public void setMessage(String message) {
        this.message = message;
    }

    public OffsetDateTime getCreatedAt() {
        return createdAt;
    }

    public void setCreatedAt(OffsetDateTime createdAt) {
        this.createdAt = createdAt;
    }

    public OffsetDateTime getAppliedAt() {
        return appliedAt;
    }

    public void setAppliedAt(OffsetDateTime appliedAt) {
        this.appliedAt = appliedAt;
    }

    public OffsetDateTime getFinishedAt() {
        return finishedAt;
    }

    public void setFinishedAt(OffsetDateTime finishedAt) {
        this.finishedAt = finishedAt;
    }
}
