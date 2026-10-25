package com.kasinlo.erp.persistence.entity;

import com.baomidou.mybatisplus.annotation.TableField;
import com.baomidou.mybatisplus.annotation.TableName;
import java.math.BigDecimal;
import java.time.LocalDate;
import java.time.OffsetDateTime;

/** Persistence mapping for sys_depts. */
@TableName("sys_depts")
public class SysDepts extends SoftDeleteEntity {
    @TableField("\"parent_id\"")
    private Long parentId;

    @TableField("\"name\"")
    private String name;

    @TableField("\"leader\"")
    private String leader;

    @TableField("\"phone\"")
    private String phone;

    @TableField("\"email\"")
    private String email;

    @TableField("\"sort\"")
    private Long sort;

    @TableField("\"status\"")
    private Long status;

    public Long getParentId() { return parentId; }
    public void setParentId(Long parentId) { this.parentId = parentId; }

    public String getName() { return name; }
    public void setName(String name) { this.name = name; }

    public String getLeader() { return leader; }
    public void setLeader(String leader) { this.leader = leader; }

    public String getPhone() { return phone; }
    public void setPhone(String phone) { this.phone = phone; }

    public String getEmail() { return email; }
    public void setEmail(String email) { this.email = email; }

    public Long getSort() { return sort; }
    public void setSort(Long sort) { this.sort = sort; }

    public Long getStatus() { return status; }
    public void setStatus(Long status) { this.status = status; }

}
