package com.kasinlo.erp.persistence.entity;

import com.baomidou.mybatisplus.annotation.TableField;
import com.baomidou.mybatisplus.annotation.TableName;
import java.math.BigDecimal;
import java.time.LocalDate;
import java.time.OffsetDateTime;

/** Persistence mapping for inv_product_specs. */
@TableName("inv_product_specs")
public class InvProductSpecs extends SoftDeleteEntity {
    @TableField("\"product_id\"")
    private Long productId;

    @TableField("\"spec\"")
    private String spec;

    @TableField("\"stock\"")
    private BigDecimal stock;

    @TableField("\"warn_stock\"")
    private BigDecimal warnStock;

    @TableField("\"sort\"")
    private Long sort;

    @TableField("\"status\"")
    private Long status;

    public Long getProductId() { return productId; }
    public void setProductId(Long productId) { this.productId = productId; }

    public String getSpec() { return spec; }
    public void setSpec(String spec) { this.spec = spec; }

    public BigDecimal getStock() { return stock; }
    public void setStock(BigDecimal stock) { this.stock = stock; }

    public BigDecimal getWarnStock() { return warnStock; }
    public void setWarnStock(BigDecimal warnStock) { this.warnStock = warnStock; }

    public Long getSort() { return sort; }
    public void setSort(Long sort) { this.sort = sort; }

    public Long getStatus() { return status; }
    public void setStatus(Long status) { this.status = status; }

}
