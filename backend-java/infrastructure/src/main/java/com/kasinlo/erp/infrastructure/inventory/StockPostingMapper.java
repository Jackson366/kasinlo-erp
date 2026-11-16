package com.kasinlo.erp.infrastructure.inventory;

import com.baomidou.mybatisplus.core.mapper.BaseMapper;
import com.kasinlo.erp.domain.inventory.StockPosting.Item;
import org.apache.ibatis.annotations.Insert;
import org.apache.ibatis.annotations.Mapper;
import org.apache.ibatis.annotations.Param;
import org.apache.ibatis.annotations.Select;
import org.apache.ibatis.annotations.Update;

import java.math.BigDecimal;
import java.time.OffsetDateTime;

@Mapper
public interface StockPostingMapper extends BaseMapper<StockOrderData> {
    @Select("SELECT COUNT(*) FROM inv_stock_orders WHERE order_no LIKE #{prefix}")
    long countNumbers(@Param("prefix") String prefix);

    @Insert("""
            INSERT INTO inv_stock_items (stock_order_id, product_id, product_name, spec_id, spec, unit_id,
                unit, rate, qty, qty_base, created_at, updated_at)
            VALUES (#{stockOrderId}, #{item.productId}, #{item.productName}, #{item.specId}, #{item.spec},
                #{item.unitId}, #{item.unit}, #{item.rate}, #{item.qty}, #{item.qtyBase}, #{at}, #{at})
            """)
    void insertLine(@Param("stockOrderId") long stockOrderId, @Param("item") Item item,
                    @Param("at") OffsetDateTime at);

    @Update("""
            UPDATE inv_product_specs SET stock = COALESCE(stock, 0) + #{delta}
            WHERE id = #{specId} AND deleted_at IS NULL
              AND (#{delta} >= 0 OR COALESCE(stock, 0) + #{delta} >= 0)
            """)
    boolean changeStock(@Param("specId") long specId, @Param("delta") BigDecimal delta);

    @Select("SELECT COALESCE(stock, 0) FROM inv_product_specs WHERE id = #{specId} AND deleted_at IS NULL")
    BigDecimal stock(@Param("specId") long specId);
}
