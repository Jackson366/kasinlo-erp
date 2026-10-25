package com.kasinlo.erp.persistence.entity;

import com.baomidou.mybatisplus.annotation.TableField;
import com.baomidou.mybatisplus.annotation.TableName;
import java.math.BigDecimal;
import java.time.LocalDate;
import java.time.OffsetDateTime;

/** Persistence mapping for sys_operation_logs. */
@TableName("sys_operation_logs")
public class SysOperationLogs extends IdentifiedEntity {
    @TableField("\"user_id\"")
    private Long userId;

    @TableField("\"username\"")
    private String username;

    @TableField("\"module\"")
    private String module;

    @TableField("\"action\"")
    private String action;

    @TableField("\"method\"")
    private String method;

    @TableField("\"path\"")
    private String path;

    @TableField("\"ip\"")
    private String ip;

    @TableField("\"user_agent\"")
    private String userAgent;

    @TableField("\"request\"")
    private String request;

    @TableField("\"status\"")
    private Long status;

    @TableField("\"latency_ms\"")
    private Long latencyMs;

    @TableField("\"error_msg\"")
    private String errorMsg;

    @TableField("\"created_at\"")
    private OffsetDateTime createdAt;

    public Long getUserId() { return userId; }
    public void setUserId(Long userId) { this.userId = userId; }

    public String getUsername() { return username; }
    public void setUsername(String username) { this.username = username; }

    public String getModule() { return module; }
    public void setModule(String module) { this.module = module; }

    public String getAction() { return action; }
    public void setAction(String action) { this.action = action; }

    public String getMethod() { return method; }
    public void setMethod(String method) { this.method = method; }

    public String getPath() { return path; }
    public void setPath(String path) { this.path = path; }

    public String getIp() { return ip; }
    public void setIp(String ip) { this.ip = ip; }

    public String getUserAgent() { return userAgent; }
    public void setUserAgent(String userAgent) { this.userAgent = userAgent; }

    public String getRequest() { return request; }
    public void setRequest(String request) { this.request = request; }

    public Long getStatus() { return status; }
    public void setStatus(Long status) { this.status = status; }

    public Long getLatencyMs() { return latencyMs; }
    public void setLatencyMs(Long latencyMs) { this.latencyMs = latencyMs; }

    public String getErrorMsg() { return errorMsg; }
    public void setErrorMsg(String errorMsg) { this.errorMsg = errorMsg; }

    public OffsetDateTime getCreatedAt() { return createdAt; }
    public void setCreatedAt(OffsetDateTime createdAt) { this.createdAt = createdAt; }

}
