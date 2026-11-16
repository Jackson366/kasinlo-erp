package com.kasinlo.erp.infrastructure.settlement;

import org.apache.ibatis.annotations.Insert;
import org.apache.ibatis.annotations.Mapper;
import org.apache.ibatis.annotations.Param;
import org.apache.ibatis.annotations.Update;

import java.math.BigDecimal;
import java.time.LocalDate;
import java.time.OffsetDateTime;

@Mapper
public interface SettlementMapper {
    @Insert("""
            INSERT INTO inv_settlements
                (order_id, plan_id, source, biz_type, party_id, amount, method, settled_at,
                 remark, created_by, created_at, updated_at)
            VALUES (#{orderId}, 0, #{source}, #{bizType}, #{partyId}, #{amount}, '银行转账',
                    #{settledAt}, '审核时直接结清', #{userId}, #{at}, #{at})
            """)
    void record(@Param("orderId") long orderId, @Param("bizType") String bizType, @Param("partyId") long partyId,
                @Param("amount") BigDecimal amount, @Param("userId") long userId,
                @Param("settledAt") LocalDate settledAt, @Param("at") OffsetDateTime at,
                @Param("source") String source);

    @Update("""
            UPDATE inv_orders SET settled_amount = COALESCE(settled_amount, 0) + #{amount}
            WHERE id = #{orderId} AND status = #{approved} AND deleted_at IS NULL
              AND COALESCE(settled_amount, 0) + #{amount} <= total_amount
            """)
    boolean settle(@Param("orderId") long orderId, @Param("amount") BigDecimal amount,
                   @Param("approved") int approved);
}
