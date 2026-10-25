package com.kasinlo.erp.persistence.entity;

import com.baomidou.mybatisplus.annotation.TableField;
import com.baomidou.mybatisplus.annotation.TableName;
import java.math.BigDecimal;
import java.time.LocalDate;
import java.time.OffsetDateTime;

/** Persistence mapping for inv_settlements. */
@TableName("inv_settlements")
public class InvSettlements extends SoftDeleteEntity {
    @TableField("\"order_id\"")
    private Long orderId;

    @TableField("\"plan_id\"")
    private Long planId;

    @TableField("\"source\"")
    private String source;

    @TableField("\"biz_type\"")
    private String bizType;

    @TableField("\"party_id\"")
    private Long partyId;

    @TableField("\"amount\"")
    private BigDecimal amount;

    @TableField("\"method\"")
    private String method;

    @TableField("\"settled_at\"")
    private LocalDate settledAt;

    @TableField("\"remark\"")
    private String remark;

    @TableField("\"created_by\"")
    private Long createdBy;

    public Long getOrderId() { return orderId; }
    public void setOrderId(Long orderId) { this.orderId = orderId; }

    public Long getPlanId() { return planId; }
    public void setPlanId(Long planId) { this.planId = planId; }

    public String getSource() { return source; }
    public void setSource(String source) { this.source = source; }

    public String getBizType() { return bizType; }
    public void setBizType(String bizType) { this.bizType = bizType; }

    public Long getPartyId() { return partyId; }
    public void setPartyId(Long partyId) { this.partyId = partyId; }

    public BigDecimal getAmount() { return amount; }
    public void setAmount(BigDecimal amount) { this.amount = amount; }

    public String getMethod() { return method; }
    public void setMethod(String method) { this.method = method; }

    public LocalDate getSettledAt() { return settledAt; }
    public void setSettledAt(LocalDate settledAt) { this.settledAt = settledAt; }

    public String getRemark() { return remark; }
    public void setRemark(String remark) { this.remark = remark; }

    public Long getCreatedBy() { return createdBy; }
    public void setCreatedBy(Long createdBy) { this.createdBy = createdBy; }

}
