package com.kasinlo.erp.persistence.entity;

import com.baomidou.mybatisplus.annotation.*;
import java.time.OffsetDateTime;

@TableName("java_request_keys")
public class RequestKey {
    @TableId(value = "request_key", type = IdType.INPUT)
    private String requestKey;
    private Long userId;
    private String fingerprint;
    private String response;
    private OffsetDateTime createdAt;

    public String getRequestKey() {
        return requestKey;
    }

    public void setRequestKey(String value) {
        requestKey = value;
    }

    public Long getUserId() {
        return userId;
    }

    public void setUserId(Long value) {
        userId = value;
    }

    public String getFingerprint() {
        return fingerprint;
    }

    public void setFingerprint(String value) {
        fingerprint = value;
    }

    public String getResponse() {
        return response;
    }

    public void setResponse(String value) {
        response = value;
    }

    public OffsetDateTime getCreatedAt() {
        return createdAt;
    }

    public void setCreatedAt(OffsetDateTime value) {
        createdAt = value;
    }
}
