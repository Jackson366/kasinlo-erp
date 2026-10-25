package com.kasinlo.erp.persistence.entity;

import com.baomidou.mybatisplus.annotation.TableField;
import com.baomidou.mybatisplus.annotation.TableName;
import java.math.BigDecimal;
import java.time.LocalDate;
import java.time.OffsetDateTime;

/** Persistence mapping for inv_products. */
@TableName("inv_products")
public class InvProducts extends SoftDeleteEntity {
    @TableField("\"code\"")
    private String code;

    @TableField("\"name\"")
    private String name;

    @TableField("\"category_id\"")
    private Long categoryId;

    @TableField("\"barcode\"")
    private String barcode;

    @TableField("\"ref_purchase_price\"")
    private BigDecimal refPurchasePrice;

    @TableField("\"ref_sale_price\"")
    private BigDecimal refSalePrice;

    @TableField("\"status\"")
    private Long status;

    @TableField("\"remark\"")
    private String remark;

    public String getCode() { return code; }
    public void setCode(String code) { this.code = code; }

    public String getName() { return name; }
    public void setName(String name) { this.name = name; }

    public Long getCategoryId() { return categoryId; }
    public void setCategoryId(Long categoryId) { this.categoryId = categoryId; }

    public String getBarcode() { return barcode; }
    public void setBarcode(String barcode) { this.barcode = barcode; }

    public BigDecimal getRefPurchasePrice() { return refPurchasePrice; }
    public void setRefPurchasePrice(BigDecimal refPurchasePrice) { this.refPurchasePrice = refPurchasePrice; }

    public BigDecimal getRefSalePrice() { return refSalePrice; }
    public void setRefSalePrice(BigDecimal refSalePrice) { this.refSalePrice = refSalePrice; }

    public Long getStatus() { return status; }
    public void setStatus(Long status) { this.status = status; }

    public String getRemark() { return remark; }
    public void setRemark(String remark) { this.remark = remark; }

}
