package com.kasinlo.erp.persistence.entity;

import com.baomidou.mybatisplus.annotation.TableField;
import com.baomidou.mybatisplus.annotation.TableName;
import java.math.BigDecimal;
import java.time.LocalDate;
import java.time.OffsetDateTime;

/** Persistence mapping for inv_contract_plans. */
@TableName("inv_contract_plans")
public class InvContractPlans extends SoftDeleteEntity {
    @TableField("\"contract_id\"")
    private Long contractId;

    @TableField("\"biz_type\"")
    private String bizType;

    @TableField("\"party_id\"")
    private Long partyId;

    @TableField("\"party_name\"")
    private String partyName;

    @TableField("\"seq\"")
    private Long seq;

    @TableField("\"name\"")
    private String name;

    @TableField("\"kind\"")
    private Long kind;

    @TableField("\"rate\"")
    private BigDecimal rate;

    @TableField("\"amount\"")
    private BigDecimal amount;

    @TableField("\"due_date\"")
    private LocalDate dueDate;

    @TableField("\"trigger_node\"")
    private String triggerNode;

    @TableField("\"settled_amount\"")
    private BigDecimal settledAmount;

    @TableField("\"remark\"")
    private String remark;

    @TableField("\"created_by\"")
    private Long createdBy;

    public Long getContractId() {
        return contractId;
    }

    public void setContractId(Long contractId) {
        this.contractId = contractId;
    }

    public String getBizType() {
        return bizType;
    }

    public void setBizType(String bizType) {
        this.bizType = bizType;
    }

    public Long getPartyId() {
        return partyId;
    }

    public void setPartyId(Long partyId) {
        this.partyId = partyId;
    }

    public String getPartyName() {
        return partyName;
    }

    public void setPartyName(String partyName) {
        this.partyName = partyName;
    }

    public Long getSeq() {
        return seq;
    }

    public void setSeq(Long seq) {
        this.seq = seq;
    }

    public String getName() {
        return name;
    }

    public void setName(String name) {
        this.name = name;
    }

    public Long getKind() {
        return kind;
    }

    public void setKind(Long kind) {
        this.kind = kind;
    }

    public BigDecimal getRate() {
        return rate;
    }

    public void setRate(BigDecimal rate) {
        this.rate = rate;
    }

    public BigDecimal getAmount() {
        return amount;
    }

    public void setAmount(BigDecimal amount) {
        this.amount = amount;
    }

    public LocalDate getDueDate() {
        return dueDate;
    }

    public void setDueDate(LocalDate dueDate) {
        this.dueDate = dueDate;
    }

    public String getTriggerNode() {
        return triggerNode;
    }

    public void setTriggerNode(String triggerNode) {
        this.triggerNode = triggerNode;
    }

    public BigDecimal getSettledAmount() {
        return settledAmount;
    }

    public void setSettledAmount(BigDecimal settledAmount) {
        this.settledAmount = settledAmount;
    }

    public String getRemark() {
        return remark;
    }

    public void setRemark(String remark) {
        this.remark = remark;
    }

    public Long getCreatedBy() {
        return createdBy;
    }

    public void setCreatedBy(Long createdBy) {
        this.createdBy = createdBy;
    }
}
