package com.kasinlo.erp.infrastructure.inventory;

import com.baomidou.mybatisplus.annotation.IdType;
import com.baomidou.mybatisplus.annotation.TableId;
import com.baomidou.mybatisplus.annotation.TableName;

import java.time.LocalDate;
import java.time.OffsetDateTime;

@TableName("inv_stock_orders")
public class StockOrderData {
    @TableId(type = IdType.AUTO)
    public Long id;
    public String orderNo;
    public String direction;
    public String sourceType;
    public long sourceId;
    public LocalDate orderDate;
    public int status;
    public String remark;
    public String salesman;
    public long createdBy;
    public OffsetDateTime createdAt;
    public OffsetDateTime updatedAt;
}
