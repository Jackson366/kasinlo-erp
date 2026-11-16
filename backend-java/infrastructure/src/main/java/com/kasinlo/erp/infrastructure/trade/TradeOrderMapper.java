package com.kasinlo.erp.infrastructure.trade;

import com.kasinlo.erp.domain.trade.TradeOrder;
import com.kasinlo.erp.domain.trade.TradeOrderGateway;
import org.apache.ibatis.annotations.Insert;
import org.apache.ibatis.annotations.Mapper;
import org.apache.ibatis.annotations.Param;
import org.apache.ibatis.annotations.Select;
import org.apache.ibatis.annotations.Update;

import java.time.OffsetDateTime;
import java.util.List;

@Mapper
public interface TradeOrderMapper extends TradeOrderGateway {
    @Override
    @Select("""
            SELECT id, order_no, biz_type, COALESCE(party_id, 0) AS party_id, order_date, status,
                   COALESCE(total_amount, 0) AS total_amount, COALESCE(settled_amount, 0) AS settled_amount,
                   COALESCE(salesman, '') AS salesman
            FROM inv_orders
            WHERE id = #{orderId} AND deleted_at IS NULL
            FOR UPDATE
            """)
    TradeOrder lock(@Param("orderId") long orderId);

    @Override
    @Select("""
            SELECT COALESCE(product_id, 0) AS product_id, COALESCE(product_name, '') AS product_name,
                   COALESCE(spec_id, 0) AS spec_id, COALESCE(spec, '') AS spec, COALESCE(unit_id, 0) AS unit_id,
                   COALESCE(unit, '') AS unit, COALESCE(rate, 1) AS rate, COALESCE(qty, 0) AS qty,
                   COALESCE(qty_base, 0) AS qty_base
            FROM inv_order_items
            WHERE order_id = #{orderId} AND deleted_at IS NULL
            ORDER BY id
            """)
    List<TradeOrder.Item> lines(@Param("orderId") long orderId);

    @Override
    @Update("""
            <script>
            <bind name="approved" value="@com.kasinlo.erp.domain.trade.TradeOrderStatus@APPROVED.getCode()"/>
            <bind name="pending" value="@com.kasinlo.erp.domain.trade.TradeOrderStatus@PENDING.getCode()"/>
            UPDATE inv_orders
            SET status = #{approved}, auditor_id = #{userId}, audited_at = #{at}, pending_since = NULL, updated_at = #{at}
            WHERE id = #{orderId} AND status = #{pending} AND deleted_at IS NULL
            </script>
            """)
    boolean approve(@Param("orderId") long orderId, @Param("userId") long userId, @Param("at") OffsetDateTime at);

    @Override
    @Insert("""
            <script>
            <bind name="action" value="@com.kasinlo.erp.domain.trade.TradeAuditActionType@AUDIT.getCode()"/>
            INSERT INTO inv_order_audits (order_id, order_no, action, actor_id, actor_name, occurred_at, stock_order_id)
            VALUES (#{orderId}, #{orderNo}, #{action}, #{userId}, #{actorName}, #{at}, #{stockOrderId})
            </script>
            """)
    void recordApproval(@Param("orderId") long orderId, @Param("orderNo") String orderNo,
                        @Param("userId") long userId, @Param("actorName") String actorName,
                        @Param("stockOrderId") long stockOrderId, @Param("at") OffsetDateTime at);
}
