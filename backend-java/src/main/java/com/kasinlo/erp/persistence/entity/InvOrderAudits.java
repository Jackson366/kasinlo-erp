package com.kasinlo.erp.persistence.entity;

import com.baomidou.mybatisplus.annotation.TableField;
import com.baomidou.mybatisplus.annotation.TableName;

import java.time.OffsetDateTime;

/** Committed audit events. No update/delete API is exposed. */
@TableName("inv_order_audits")
public class InvOrderAudits extends IdentifiedEntity {
    @TableField("\"order_id\"")
    private Long orderId;

    public Long getOrderId() {
        return orderId;
    }

    public void setOrderId(Long value) {
        this.orderId = value;
    }

    @TableField("\"order_no\"")
    private String orderNo;

    public String getOrderNo() {
        return orderNo;
    }

    public void setOrderNo(String value) {
        this.orderNo = value;
    }

    @TableField("\"action\"")
    private String action;

    public String getAction() {
        return action;
    }

    public void setAction(String value) {
        this.action = value;
    }

    @TableField("\"actor_id\"")
    private Long actorId;

    public Long getActorId() {
        return actorId;
    }

    public void setActorId(Long value) {
        this.actorId = value;
    }

    @TableField("\"actor_name\"")
    private String actorName;

    public String getActorName() {
        return actorName;
    }

    public void setActorName(String value) {
        this.actorName = value;
    }

    @TableField("\"occurred_at\"")
    private OffsetDateTime occurredAt;

    public OffsetDateTime getOccurredAt() {
        return occurredAt;
    }

    public void setOccurredAt(OffsetDateTime value) {
        this.occurredAt = value;
    }

    @TableField("\"stock_order_id\"")
    private Long stockOrderId;

    public Long getStockOrderId() {
        return stockOrderId;
    }

    public void setStockOrderId(Long value) {
        this.stockOrderId = value;
    }
}
