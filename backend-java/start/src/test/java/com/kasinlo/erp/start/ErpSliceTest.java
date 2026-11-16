package com.kasinlo.erp.start;

import com.alibaba.cola.exception.BizException;
import com.kasinlo.erp.app.trade.ApproveTradeOrder.Actor;
import com.kasinlo.erp.app.trade.ApproveTradeOrder;
import com.kasinlo.erp.common.error.ErpException;
import com.kasinlo.erp.domain.trade.TradeErrorType;
import com.nimbusds.jose.JOSEException;
import com.nimbusds.jose.JWSAlgorithm;
import com.nimbusds.jose.JWSHeader;
import com.nimbusds.jose.crypto.MACSigner;
import com.nimbusds.jwt.JWTClaimsSet;
import com.nimbusds.jwt.SignedJWT;
import jakarta.validation.ConstraintViolationException;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.webmvc.test.autoconfigure.AutoConfigureMockMvc;
import org.springframework.dao.DataIntegrityViolationException;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.jdbc.support.GeneratedKeyHolder;
import org.springframework.test.context.ActiveProfiles;
import org.springframework.test.web.servlet.MockMvc;

import java.math.BigDecimal;
import java.sql.PreparedStatement;
import java.sql.Statement;
import java.time.Clock;
import java.time.Instant;
import java.util.Date;
import java.util.List;
import java.util.concurrent.CyclicBarrier;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.header;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

@SpringBootTest
@AutoConfigureMockMvc
@ActiveProfiles("test")
class ErpSliceTest {
    private static final String INVENTORY = "/api/v1/internal/open-platform/inventory";

    @Autowired MockMvc mvc;
    @Autowired JdbcTemplate jdbc;
    @Autowired ApproveTradeOrder approve;
    @Autowired Clock clock;
    @Value("${erp.platform.secret}") String secret;
    @Value("${erp.platform.issuer}") String issuer;
    @Value("${erp.platform.audience}") String audience;

    private long userId;

    @BeforeEach
    void prepareDatabase() {
        for (String table : List.of("inv_order_audits", "inv_settlements", "inv_stock_items", "inv_stock_orders",
                "inv_order_items", "inv_orders", "inv_product_units", "inv_product_specs", "inv_products",
                "sys_role_menus", "sys_user_roles", "sys_menus", "sys_roles", "sys_users")) {
            jdbc.update("DELETE FROM " + table);
        }
        userId = insert("INSERT INTO sys_users (username, status, token_epoch) VALUES ('operator', 1, 2)");
    }

    @Test
    void approvalValidatesIncomingParametersBeforeLookingUpTheOrder() {
        assertThatThrownBy(() -> approve.execute(0, new Actor(userId, "采购员"), true))
                .isInstanceOfSatisfying(ConstraintViolationException.class, error ->
                        assertThat(error.getConstraintViolations())
                                .extracting(violation -> violation.getPropertyPath().toString())
                                .containsExactly("execute.orderId"));

        assertThatThrownBy(() -> approve.execute(123L, new Actor(0, null), true))
                .isInstanceOfSatisfying(ConstraintViolationException.class, error ->
                        assertThat(error.getConstraintViolations())
                                .extracting(violation -> violation.getPropertyPath().toString())
                                .containsExactlyInAnyOrder("execute.actor.id", "execute.actor.name"));
    }

    @Test
    void platformInventoryPreservesPageSearchAndPublicFields() throws Exception {
        grantInventory();
        Fixture matching = product("P_100", "主商品", "φ20", "12.5000", 1, 1);
        product("PX100", "另一商品", "φ25", "50", 1, 1);
        product("P_100-disabled", "停用商品", "φ20", "3", 0, 1);
        product("P_100-disabled-spec", "停用规格", "φ20", "3", 1, 0);
        Fixture removed = product("P_100-removed", "已删除商品", "φ20", "3", 1, 1);
        jdbc.update("UPDATE inv_products SET deleted_at = CURRENT_TIMESTAMP WHERE id = ?", removed.productId());

        mvc.perform(get(INVENTORY).param("keyword", "p_100").param("pageSize", "1")
                        .header("Authorization", token(userId, 2, "inventory.read")))
                .andExpect(status().isOk())
                .andExpect(header().string("Cache-Control", "no-store"))
                .andExpect(jsonPath("$.code").value(0))
                .andExpect(jsonPath("$.data.page").value(1))
                .andExpect(jsonPath("$.data.pageSize").value(1))
                .andExpect(jsonPath("$.data.total").value(1))
                .andExpect(jsonPath("$.data.list[0].specId").value(matching.specId()))
                .andExpect(jsonPath("$.data.list[0].productCode").value("P_100"))
                .andExpect(jsonPath("$.data.list[0].unit").value("件"))
                .andExpect(jsonPath("$.data.list[0].stock").value(12.5))
                .andExpect(jsonPath("$.data.list[0].purchasePrice").doesNotExist());

        mvc.perform(get(INVENTORY).param("page", "9223372036854775807").param("pageSize", "300")
                        .header("Authorization", token(userId, 2, "inventory.read")))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.data.page").value(5001))
                .andExpect(jsonPath("$.data.pageSize").value(200))
                .andExpect(jsonPath("$.data.list").isEmpty());
    }

    @Test
    void platformInventoryRequiresScopePermissionAndCurrentToken() throws Exception {
        mvc.perform(get(INVENTORY)).andExpect(status().isUnauthorized())
                .andExpect(jsonPath("$.code").value(40100))
                .andExpect(jsonPath("$.message").value("登录状态已失效，请重新登录"));
        mvc.perform(get(INVENTORY).header("Authorization", token(userId, 2, "sales.read")))
                .andExpect(status().isForbidden())
                .andExpect(jsonPath("$.code").value(40300))
                .andExpect(jsonPath("$.message").value("没有操作权限"));
        mvc.perform(get(INVENTORY).header("Authorization", token(userId, 2, "inventory.read")))
                .andExpect(status().isForbidden())
                .andExpect(jsonPath("$.code").value(40300))
                .andExpect(jsonPath("$.message").value("缺少权限: inv:stock:list"));

        grantInventory();
        mvc.perform(get(INVENTORY).header("Authorization", token(userId, 1, "inventory.read")))
                .andExpect(status().isUnauthorized()).andExpect(jsonPath("$.code").value(40105));
        mvc.perform(get(INVENTORY).header("Authorization", token(userId, 2, "inventory.read",
                        issuer, audience, clock.instant().minusSeconds(80))))
                .andExpect(status().isUnauthorized());
        mvc.perform(get(INVENTORY).header("Authorization", token(userId, 2, "inventory.read",
                        issuer, "another-audience", clock.instant().minusSeconds(1))))
                .andExpect(status().isUnauthorized());

        jdbc.update("UPDATE sys_users SET status = 0 WHERE id = ?", userId);
        mvc.perform(get(INVENTORY).header("Authorization", token(userId, 2, "inventory.read")))
                .andExpect(status().isUnauthorized()).andExpect(jsonPath("$.code").value(40102));
    }

    @Test
    void purchaseApprovalPostsSnapshotSettlesAndAuditsInOneCommit() {
        Fixture spec = product("P-1", "当前档案", "新规格", "10.0000", 1, 1);
        long order = order("purchase", "31.27");
        line(order, spec, "1.20", "2.4996");
        line(order, spec, "2.00", "4.1660");

        approve.execute(order, new Actor(userId, "采购员"), true);

        assertThat(stock(spec)).isEqualByComparingTo("16.6656");
        assertThat(jdbc.queryForObject("SELECT status FROM inv_orders WHERE id = ?", Integer.class, order)).isEqualTo(2);
        assertThat(jdbc.queryForObject("SELECT settled_amount FROM inv_orders WHERE id = ?", BigDecimal.class, order))
                .isEqualByComparingTo("31.27");
        assertThat(jdbc.queryForObject("SELECT amount FROM inv_settlements WHERE order_id = ?", BigDecimal.class, order))
                .isEqualByComparingTo("31.27");
        assertThat(jdbc.queryForObject("SELECT direction FROM inv_stock_orders WHERE source_id = ?", String.class, order))
                .isEqualTo("in");
        assertThat(jdbc.queryForList("SELECT product_name FROM inv_stock_items", String.class))
                .containsExactly("原商品名称", "原商品名称");
        assertThat(jdbc.queryForObject("""
                SELECT audit.actor_name FROM inv_order_audits audit JOIN inv_stock_orders stock
                ON stock.id = audit.stock_order_id AND stock.source_id = audit.order_id
                WHERE audit.order_id = ?
                """, String.class, order)).isEqualTo("采购员");
    }

    @Test
    void auditFailureRollsBackStockSettlementAndApproval() {
        Fixture spec = product("P-1", "商品", "规格", "10.0000", 1, 1);
        long order = order("purchase", "31.27");
        line(order, spec, "1.20", "2.4996");
        jdbc.execute("ALTER TABLE inv_order_audits ADD CONSTRAINT block_test_actor CHECK (actor_name <> 'blocked')");
        try {
            assertThatThrownBy(() -> approve.execute(order, new Actor(userId, "blocked"), true))
                    .isInstanceOf(DataIntegrityViolationException.class)
                    .hasMessageContaining("block_test_actor");
        } finally {
            jdbc.execute("ALTER TABLE inv_order_audits DROP CONSTRAINT block_test_actor");
        }

        assertThat(stock(spec)).isEqualByComparingTo("10.0000");
        assertPendingWithoutPosting(order);
    }

    @Test
    void salesOutflowRejectsAggregateShortageAndRevertsEarlierSpecUpdate() {
        Fixture first = product("P-1", "充足商品", "规格1", "10", 1, 1);
        Fixture scarce = product("P-2", "不足商品", "规格2", "3", 1, 1);
        long order = order("sales", "31.27");
        line(order, first, "1.00", "2.083");
        line(order, scarce, "1.00", "2.083");
        line(order, scarce, "1.00", "2.083");

        assertThatThrownBy(() -> approve.execute(order, new Actor(userId, "销售员"), true))
                .isInstanceOf(BizException.class).hasMessageContaining("库存不足");
        assertThat(stock(first)).isEqualByComparingTo("10");
        assertThat(stock(scarce)).isEqualByComparingTo("3");
        assertPendingWithoutPosting(order);
    }

    @Test
    void rejectedLegacyCodesDoNotTriggerPosting() {
        long order = order("unknown", "10");
        Actor actor = new Actor(userId, "采购员");
        assertThatThrownBy(() -> approve.execute(order, actor, true))
                .isInstanceOfSatisfying(ErpException.class,
                        exception -> assertThat(exception.error()).isEqualTo(TradeErrorType.ORDER_TYPE_INVALID));
        assertPendingWithoutPosting(order);

        jdbc.update("UPDATE inv_orders SET biz_type = 'purchase', status = 3 WHERE id = ?", order);
        assertThatThrownBy(() -> approve.execute(order, actor, true))
                .isInstanceOfSatisfying(ErpException.class,
                        exception -> assertThat(exception.error()).isEqualTo(TradeErrorType.ORDER_NOT_PENDING));
        assertThat(count("inv_stock_orders")).isZero();
    }

    @Test
    void concurrentApprovalPostsOnce() throws Exception {
        Fixture spec = product("P-1", "商品", "规格", "10", 1, 1);
        long order = order("purchase", "0.00");
        line(order, spec, "1.00", "2.083");
        CyclicBarrier gate = new CyclicBarrier(2);

        try (var executor = Executors.newFixedThreadPool(2)) {
            var first = executor.submit(() -> approveAfter(gate, order));
            var second = executor.submit(() -> approveAfter(gate, order));
            assertThat(List.of(first.get(10, TimeUnit.SECONDS), second.get(10, TimeUnit.SECONDS)))
                    .containsExactlyInAnyOrder("approved", "40900");
        }

        assertThat(stock(spec)).isEqualByComparingTo("12.0830");
        assertThat(count("inv_stock_orders")).isOne();
        assertThat(count("inv_order_audits")).isOne();
        assertThat(count("inv_settlements")).isZero();
    }

    private String approveAfter(CyclicBarrier gate, long order) throws Exception {
        gate.await(5, TimeUnit.SECONDS);
        try {
            approve.execute(order, new Actor(userId, "采购员"), true);
            return "approved";
        } catch (BizException e) {
            return e.getErrCode();
        }
    }

    private void assertPendingWithoutPosting(long order) {
        assertThat(jdbc.queryForObject("SELECT status FROM inv_orders WHERE id = ?", Integer.class, order)).isEqualTo(1);
        assertThat(jdbc.queryForObject("SELECT auditor_id FROM inv_orders WHERE id = ?", Long.class, order)).isNull();
        assertThat(jdbc.queryForObject("SELECT settled_amount FROM inv_orders WHERE id = ?", BigDecimal.class, order))
                .isEqualByComparingTo("0");
        for (String table : List.of("inv_stock_orders", "inv_stock_items", "inv_settlements", "inv_order_audits")) {
            assertThat(count(table)).as(table).isZero();
        }
    }

    private int count(String table) {
        return jdbc.queryForObject("SELECT COUNT(*) FROM " + table, Integer.class);
    }

    private void grantInventory() {
        long role = insert("INSERT INTO sys_roles (code, status) VALUES ('stock-reader', 1)");
        long menu = insert("INSERT INTO sys_menus (permission, status) VALUES ('inv:stock:list', 1)");
        jdbc.update("INSERT INTO sys_user_roles (user_id, role_id) VALUES (?, ?)", userId, role);
        jdbc.update("INSERT INTO sys_role_menus (role_id, menu_id) VALUES (?, ?)", role, menu);
    }

    private String token(long user, long epoch, String scope) throws JOSEException {
        return token(user, epoch, scope, issuer, audience, clock.instant().minusSeconds(1));
    }

    private String token(long user, long epoch, String scope, String issuer, String audience, Instant issued)
            throws JOSEException {
        var claims = new JWTClaimsSet.Builder().subject(Long.toString(user)).issuer(issuer).audience(audience)
                .issueTime(Date.from(issued)).expirationTime(Date.from(issued.plusSeconds(45)))
                .claim("scope", scope).claim("grant_id", "fixture-grant").claim("epoch", epoch).build();
        var jwt = new SignedJWT(new JWSHeader(JWSAlgorithm.HS256), claims);
        jwt.sign(new MACSigner(secret));
        return "Bearer " + jwt.serialize();
    }

    private Fixture product(String code, String name, String specName, String stock, int productStatus, int specStatus) {
        long product = insert("INSERT INTO inv_products (code, name, status) VALUES (?, ?, ?)", code, name, productStatus);
        long spec = insert("INSERT INTO inv_product_specs (product_id, spec, stock, status) VALUES (?, ?, ?, ?)",
                product, specName, new BigDecimal(stock), specStatus);
        long unit = insert("INSERT INTO inv_product_units (spec_id, unit, is_base) VALUES (?, '件', TRUE)", spec);
        return new Fixture(product, spec, unit);
    }

    private long order(String type, String amount) {
        return insert("""
                INSERT INTO inv_orders (order_no, biz_type, order_date, status, total_amount, salesman,
                                        created_by, pending_since)
                VALUES ('order-for-approval', ?, DATE '2026-11-15', 1, ?, '业务员', ?, CURRENT_TIMESTAMP)
                """, type, new BigDecimal(amount), userId);
    }

    private void line(long order, Fixture spec, String qty, String quantityBase) {
        jdbc.update("""
                INSERT INTO inv_order_items (order_id, product_id, product_name, spec_id, spec, unit_id, unit,
                                             rate, qty, qty_base)
                VALUES (?, ?, '原商品名称', ?, '原规格', ?, '箱', 2.0830, ?, ?)
                """, order, spec.productId(), spec.specId(), spec.unitId(),
                new BigDecimal(qty), new BigDecimal(quantityBase));
    }

    private BigDecimal stock(Fixture spec) {
        return jdbc.queryForObject("SELECT stock FROM inv_product_specs WHERE id = ?", BigDecimal.class, spec.specId());
    }

    private long insert(String sql, Object... values) {
        var key = new GeneratedKeyHolder();
        jdbc.update(connection -> {
            PreparedStatement statement = connection.prepareStatement(sql, Statement.RETURN_GENERATED_KEYS);
            for (int i = 0; i < values.length; i++) {
                statement.setObject(i + 1, values[i]);
            }
            return statement;
        }, key);
        return key.getKey().longValue();
    }

    private record Fixture(long productId, long specId, long unitId) {}
}
