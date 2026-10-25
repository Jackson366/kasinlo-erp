package com.kasinlo.erp.persistence.entity;

import com.baomidou.mybatisplus.annotation.TableField;
import com.baomidou.mybatisplus.annotation.TableName;
import java.math.BigDecimal;
import java.time.LocalDate;
import java.time.OffsetDateTime;

/** Persistence mapping for sys_users. */
@TableName("sys_users")
public class SysUsers extends SoftDeleteEntity {
    @TableField("\"username\"")
    private String username;

    @TableField("\"password\"")
    private String password;

    @TableField("\"nickname\"")
    private String nickname;

    @TableField("\"email\"")
    private String email;

    @TableField("\"phone\"")
    private String phone;

    @TableField("\"avatar\"")
    private String avatar;

    @TableField("\"dept_id\"")
    private Long deptId;

    @TableField("\"status\"")
    private Long status;

    @TableField("\"token_epoch\"")
    private Long tokenEpoch;

    @TableField("\"remark\"")
    private String remark;

    @TableField("\"last_login_at\"")
    private OffsetDateTime lastLoginAt;

    @TableField("\"last_login_ip\"")
    private String lastLoginIp;

    public String getUsername() { return username; }
    public void setUsername(String username) { this.username = username; }

    public String getPassword() { return password; }
    public void setPassword(String password) { this.password = password; }

    public String getNickname() { return nickname; }
    public void setNickname(String nickname) { this.nickname = nickname; }

    public String getEmail() { return email; }
    public void setEmail(String email) { this.email = email; }

    public String getPhone() { return phone; }
    public void setPhone(String phone) { this.phone = phone; }

    public String getAvatar() { return avatar; }
    public void setAvatar(String avatar) { this.avatar = avatar; }

    public Long getDeptId() { return deptId; }
    public void setDeptId(Long deptId) { this.deptId = deptId; }

    public Long getStatus() { return status; }
    public void setStatus(Long status) { this.status = status; }

    public Long getTokenEpoch() { return tokenEpoch; }
    public void setTokenEpoch(Long tokenEpoch) { this.tokenEpoch = tokenEpoch; }

    public String getRemark() { return remark; }
    public void setRemark(String remark) { this.remark = remark; }

    public OffsetDateTime getLastLoginAt() { return lastLoginAt; }
    public void setLastLoginAt(OffsetDateTime lastLoginAt) { this.lastLoginAt = lastLoginAt; }

    public String getLastLoginIp() { return lastLoginIp; }
    public void setLastLoginIp(String lastLoginIp) { this.lastLoginIp = lastLoginIp; }

}
