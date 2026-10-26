package com.kasinlo.erp.persistence.entity;

import com.baomidou.mybatisplus.annotation.TableField;
import com.baomidou.mybatisplus.annotation.TableName;
import java.math.BigDecimal;
import java.time.LocalDate;
import java.time.OffsetDateTime;

/** Persistence mapping for inv_stock_items. */
@TableName("inv_stock_items")
public class InvStockItems extends SoftDeleteEntity {
    @TableField("\"stock_order_id\"")
    private Long stockOrderId;

    @TableField("\"product_id\"")
    private Long productId;

    @TableField("\"product_name\"")
    private String productName;

    @TableField("\"spec_id\"")
    private Long specId;

    @TableField("\"spec\"")
    private String spec;

    @TableField("\"unit_id\"")
    private Long unitId;

    @TableField("\"unit\"")
    private String unit;

    @TableField("\"rate\"")
    private BigDecimal rate;

    @TableField("\"qty\"")
    private BigDecimal qty;

    @TableField("\"qty_base\"")
    private BigDecimal qtyBase;

    public Long getStockOrderId() {
        return stockOrderId;
    }

    public void setStockOrderId(Long stockOrderId) {
        this.stockOrderId = stockOrderId;
    }

    public Long getProductId() {
        return productId;
    }

    public void setProductId(Long productId) {
        this.productId = productId;
    }

    public String getProductName() {
        return productName;
    }

    public void setProductName(String productName) {
        this.productName = productName;
    }

    public Long getSpecId() {
        return specId;
    }

    public void setSpecId(Long specId) {
        this.specId = specId;
    }

    public String getSpec() {
        return spec;
    }

    public void setSpec(String spec) {
        this.spec = spec;
    }

    public Long getUnitId() {
        return unitId;
    }

    public void setUnitId(Long unitId) {
        this.unitId = unitId;
    }

    public String getUnit() {
        return unit;
    }

    public void setUnit(String unit) {
        this.unit = unit;
    }

    public BigDecimal getRate() {
        return rate;
    }

    public void setRate(BigDecimal rate) {
        this.rate = rate;
    }

    public BigDecimal getQty() {
        return qty;
    }

    public void setQty(BigDecimal qty) {
        this.qty = qty;
    }

    public BigDecimal getQtyBase() {
        return qtyBase;
    }

    public void setQtyBase(BigDecimal qtyBase) {
        this.qtyBase = qtyBase;
    }
}
