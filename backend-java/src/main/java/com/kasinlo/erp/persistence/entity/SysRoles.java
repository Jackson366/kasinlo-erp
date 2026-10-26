package com.kasinlo.erp.persistence.entity;

import com.baomidou.mybatisplus.annotation.TableField;
import com.baomidou.mybatisplus.annotation.TableName;
import java.math.BigDecimal;
import java.time.LocalDate;
import java.time.OffsetDateTime;

/** Persistence mapping for sys_roles. */
@TableName("sys_roles")
public class SysRoles extends SoftDeleteEntity {
    @TableField("\"name\"")
    private String name;

    @TableField("\"code\"")
    private String code;

    @TableField("\"sort\"")
    private Long sort;

    @TableField("\"status\"")
    private Long status;

    @TableField("\"data_scope\"")
    private Long dataScope;

    @TableField("\"is_builtin\"")
    private Boolean isBuiltin;

    @TableField("\"remark\"")
    private String remark;

    public String getName() {
        return name;
    }

    public void setName(String name) {
        this.name = name;
    }

    public String getCode() {
        return code;
    }

    public void setCode(String code) {
        this.code = code;
    }

    public Long getSort() {
        return sort;
    }

    public void setSort(Long sort) {
        this.sort = sort;
    }

    public Long getStatus() {
        return status;
    }

    public void setStatus(Long status) {
        this.status = status;
    }

    public Long getDataScope() {
        return dataScope;
    }

    public void setDataScope(Long dataScope) {
        this.dataScope = dataScope;
    }

    public Boolean getIsBuiltin() {
        return isBuiltin;
    }

    public void setIsBuiltin(Boolean isBuiltin) {
        this.isBuiltin = isBuiltin;
    }

    public String getRemark() {
        return remark;
    }

    public void setRemark(String remark) {
        this.remark = remark;
    }
}
