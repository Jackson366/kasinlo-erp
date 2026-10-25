package com.kasinlo.erp.persistence.entity;

import com.baomidou.mybatisplus.annotation.TableField;
import com.baomidou.mybatisplus.annotation.TableName;
import java.math.BigDecimal;
import java.time.LocalDate;
import java.time.OffsetDateTime;

/** Persistence mapping for inv_product_units. */
@TableName("inv_product_units")
public class InvProductUnits extends SoftDeleteEntity {
    @TableField("\"spec_id\"")
    private Long specId;

    @TableField("\"unit\"")
    private String unit;

    @TableField("\"rate\"")
    private BigDecimal rate;

    @TableField("\"is_base\"")
    private Boolean isBase;

    @TableField("\"purchase_price\"")
    private BigDecimal purchasePrice;

    @TableField("\"sale_price\"")
    private BigDecimal salePrice;

    @TableField("\"sort\"")
    private Long sort;

    @TableField("\"status\"")
    private Long status;

    public Long getSpecId() { return specId; }
    public void setSpecId(Long specId) { this.specId = specId; }

    public String getUnit() { return unit; }
    public void setUnit(String unit) { this.unit = unit; }

    public BigDecimal getRate() { return rate; }
    public void setRate(BigDecimal rate) { this.rate = rate; }

    public Boolean getIsBase() { return isBase; }
    public void setIsBase(Boolean isBase) { this.isBase = isBase; }

    public BigDecimal getPurchasePrice() { return purchasePrice; }
    public void setPurchasePrice(BigDecimal purchasePrice) { this.purchasePrice = purchasePrice; }

    public BigDecimal getSalePrice() { return salePrice; }
    public void setSalePrice(BigDecimal salePrice) { this.salePrice = salePrice; }

    public Long getSort() { return sort; }
    public void setSort(Long sort) { this.sort = sort; }

    public Long getStatus() { return status; }
    public void setStatus(Long status) { this.status = status; }

}
