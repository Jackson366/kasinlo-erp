package com.kasinlo.erp.persistence.mapper;

import java.math.BigDecimal;
import org.apache.ibatis.annotations.Param;
import org.apache.ibatis.annotations.Update;

public interface StockMutationMapper {
    @Update("""
        UPDATE inv_product_specs SET stock = stock + #{delta}
        WHERE id = #{id} AND deleted_at IS NULL AND stock + #{delta} >= 0
        """)
    int adjust(@Param("id") long id, @Param("delta") BigDecimal delta);
}
