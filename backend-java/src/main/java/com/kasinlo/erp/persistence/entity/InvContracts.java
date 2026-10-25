package com.kasinlo.erp.persistence.entity;

import com.baomidou.mybatisplus.annotation.TableField;
import com.baomidou.mybatisplus.annotation.TableName;
import java.math.BigDecimal;
import java.time.LocalDate;
import java.time.OffsetDateTime;

/** Persistence mapping for inv_contracts. */
@TableName("inv_contracts")
public class InvContracts extends SoftDeleteEntity {
    @TableField("\"contract_no\"")
    private String contractNo;

    @TableField("\"name\"")
    private String name;

    @TableField("\"biz_type\"")
    private String bizType;

    @TableField("\"party_id\"")
    private Long partyId;

    @TableField("\"party_name\"")
    private String partyName;

    @TableField("\"sign_date\"")
    private LocalDate signDate;

    @TableField("\"start_date\"")
    private LocalDate startDate;

    @TableField("\"end_date\"")
    private LocalDate endDate;

    @TableField("\"amount\"")
    private BigDecimal amount;

    @TableField("\"status\"")
    private Long status;

    @TableField("\"salesman\"")
    private String salesman;

    @TableField("\"remark\"")
    private String remark;

    @TableField("\"created_by\"")
    private Long createdBy;

    @TableField("\"audited_at\"")
    private OffsetDateTime auditedAt;

    @TableField("\"auditor\"")
    private String auditor;

    @TableField("\"accepted_at\"")
    private LocalDate acceptedAt;

    @TableField("\"accept_result\"")
    private String acceptResult;

    @TableField("\"warranty_months\"")
    private Long warrantyMonths;

    @TableField("\"warranty_end_at\"")
    private LocalDate warrantyEndAt;

    @TableField("\"retention_rate\"")
    private BigDecimal retentionRate;

    @TableField("\"retention_amount\"")
    private BigDecimal retentionAmount;

    public String getContractNo() { return contractNo; }
    public void setContractNo(String contractNo) { this.contractNo = contractNo; }

    public String getName() { return name; }
    public void setName(String name) { this.name = name; }

    public String getBizType() { return bizType; }
    public void setBizType(String bizType) { this.bizType = bizType; }

    public Long getPartyId() { return partyId; }
    public void setPartyId(Long partyId) { this.partyId = partyId; }

    public String getPartyName() { return partyName; }
    public void setPartyName(String partyName) { this.partyName = partyName; }

    public LocalDate getSignDate() { return signDate; }
    public void setSignDate(LocalDate signDate) { this.signDate = signDate; }

    public LocalDate getStartDate() { return startDate; }
    public void setStartDate(LocalDate startDate) { this.startDate = startDate; }

    public LocalDate getEndDate() { return endDate; }
    public void setEndDate(LocalDate endDate) { this.endDate = endDate; }

    public BigDecimal getAmount() { return amount; }
    public void setAmount(BigDecimal amount) { this.amount = amount; }

    public Long getStatus() { return status; }
    public void setStatus(Long status) { this.status = status; }

    public String getSalesman() { return salesman; }
    public void setSalesman(String salesman) { this.salesman = salesman; }

    public String getRemark() { return remark; }
    public void setRemark(String remark) { this.remark = remark; }

    public Long getCreatedBy() { return createdBy; }
    public void setCreatedBy(Long createdBy) { this.createdBy = createdBy; }

    public OffsetDateTime getAuditedAt() { return auditedAt; }
    public void setAuditedAt(OffsetDateTime auditedAt) { this.auditedAt = auditedAt; }

    public String getAuditor() { return auditor; }
    public void setAuditor(String auditor) { this.auditor = auditor; }

    public LocalDate getAcceptedAt() { return acceptedAt; }
    public void setAcceptedAt(LocalDate acceptedAt) { this.acceptedAt = acceptedAt; }

    public String getAcceptResult() { return acceptResult; }
    public void setAcceptResult(String acceptResult) { this.acceptResult = acceptResult; }

    public Long getWarrantyMonths() { return warrantyMonths; }
    public void setWarrantyMonths(Long warrantyMonths) { this.warrantyMonths = warrantyMonths; }

    public LocalDate getWarrantyEndAt() { return warrantyEndAt; }
    public void setWarrantyEndAt(LocalDate warrantyEndAt) { this.warrantyEndAt = warrantyEndAt; }

    public BigDecimal getRetentionRate() { return retentionRate; }
    public void setRetentionRate(BigDecimal retentionRate) { this.retentionRate = retentionRate; }

    public BigDecimal getRetentionAmount() { return retentionAmount; }
    public void setRetentionAmount(BigDecimal retentionAmount) { this.retentionAmount = retentionAmount; }

}
