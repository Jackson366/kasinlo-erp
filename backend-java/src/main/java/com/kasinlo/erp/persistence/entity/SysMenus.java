package com.kasinlo.erp.persistence.entity;

import com.baomidou.mybatisplus.annotation.TableField;
import com.baomidou.mybatisplus.annotation.TableName;
import java.math.BigDecimal;
import java.time.LocalDate;
import java.time.OffsetDateTime;

/** Persistence mapping for sys_menus. */
@TableName("sys_menus")
public class SysMenus extends SoftDeleteEntity {
    @TableField("\"parent_id\"")
    private Long parentId;

    @TableField("\"name\"")
    private String name;

    @TableField("\"path\"")
    private String path;

    @TableField("\"component\"")
    private String component;

    @TableField("\"redirect\"")
    private String redirect;

    @TableField("\"title\"")
    private String title;

    @TableField("\"icon\"")
    private String icon;

    @TableField("\"type\"")
    private String type;

    @TableField("\"permission\"")
    private String permission;

    @TableField("\"sort\"")
    private Long sort;

    @TableField("\"hidden\"")
    private Boolean hidden;

    @TableField("\"keep_alive\"")
    private Boolean keepAlive;

    @TableField("\"is_external\"")
    private Boolean isExternal;

    @TableField("\"status\"")
    private Long status;

    public Long getParentId() { return parentId; }
    public void setParentId(Long parentId) { this.parentId = parentId; }

    public String getName() { return name; }
    public void setName(String name) { this.name = name; }

    public String getPath() { return path; }
    public void setPath(String path) { this.path = path; }

    public String getComponent() { return component; }
    public void setComponent(String component) { this.component = component; }

    public String getRedirect() { return redirect; }
    public void setRedirect(String redirect) { this.redirect = redirect; }

    public String getTitle() { return title; }
    public void setTitle(String title) { this.title = title; }

    public String getIcon() { return icon; }
    public void setIcon(String icon) { this.icon = icon; }

    public String getType() { return type; }
    public void setType(String type) { this.type = type; }

    public String getPermission() { return permission; }
    public void setPermission(String permission) { this.permission = permission; }

    public Long getSort() { return sort; }
    public void setSort(Long sort) { this.sort = sort; }

    public Boolean getHidden() { return hidden; }
    public void setHidden(Boolean hidden) { this.hidden = hidden; }

    public Boolean getKeepAlive() { return keepAlive; }
    public void setKeepAlive(Boolean keepAlive) { this.keepAlive = keepAlive; }

    public Boolean getIsExternal() { return isExternal; }
    public void setIsExternal(Boolean isExternal) { this.isExternal = isExternal; }

    public Long getStatus() { return status; }
    public void setStatus(Long status) { this.status = status; }

}
