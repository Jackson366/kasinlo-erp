package com.kasinlo.erp.module.inv;

import static com.kasinlo.erp.common.Values.*;

import static org.junit.jupiter.api.Assertions.*;

import com.kasinlo.erp.common.BusinessException;
import com.kasinlo.erp.module.system.IdentityService.Actor;
import com.kasinlo.erp.persistence.ErpRepository;

import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.transaction.PlatformTransactionManager;
import org.springframework.transaction.support.TransactionTemplate;

import java.util.*;
import java.util.concurrent.*;

@SpringBootTest(
        properties = {
            "spring.datasource.url=jdbc:h2:mem:audit-tests;MODE=PostgreSQL;DATABASE_TO_LOWER=TRUE;DB_CLOSE_DELAY=-1",
            "erp.demo-data=false"
        })
class TradeAuditTest {
    @org.springframework.test.context.bean.override.mockito.MockitoSpyBean ErpRepository db;
    @Autowired TradeService trade;
    @Autowired PlatformTransactionManager transactions;

    private void tx(Runnable operation) {
        new TransactionTemplate(transactions).executeWithoutResult(status -> operation.run());
    }

    private record Fixture(long order, long spec, Actor actor) {}

    private Fixture fixture() {
        String name = "audit-" + UUID.randomUUID().toString().substring(0, 16);
        long user = db.insert("sys_users", map("username", name, "nickname", "审核测试员"));
        var actor = new Actor(user, db.find("sys_users", user), List.of(), Set.of(), true, 1);
        long party = db.insert("inv_suppliers", map("name", name, "code", name));
        long product = db.insert("inv_products", map("name", name, "code", name));
        long spec =
                db.insert(
                        "inv_product_specs", map("productId", product, "spec", "test", "stock", 0));
        long unit = db.insert("inv_product_units", map("specId", spec, "unit", "件", "rate", 1));
        long[] order = {0};
        tx(
                () ->
                        order[0] =
                                trade.createOrder(
                                        map(
                                                "bizType",
                                                "purchase",
                                                "partyId",
                                                party,
                                                "orderDate",
                                                "2026-10-27",
                                                "items",
                                                List.of(
                                                        map(
                                                                "productId",
                                                                product,
                                                                "specId",
                                                                spec,
                                                                "unitId",
                                                                unit,
                                                                "qty",
                                                                10,
                                                                "price",
                                                                2))),
                                        actor,
                                        0));
        return new Fixture(order[0], spec, actor);
    }

    @Test
    void everyCommittedTransitionRetainsHistoryAndDownstream() {
        var f = fixture();
        tx(() -> trade.audit(f.order(), map("status", 2), f.actor()));
        var first = trade.order(f.order(), f.actor());
        assertEquals(f.actor().id(), n(first, "auditorId"));
        assertNotNull(first.get("auditedAt"));
        assertEquals(true, first.get("selfAudit"));
        assertEquals(1, rows(first.get("auditHistory")).size());
        assertEquals(1, rows(((Map<?, ?>) first.get("downstream")).get("stockOrders")).size());
        var stock = db.where("inv_stock_orders", "sourceId", f.order()).get(0);
        assertThrows(
                BusinessException.class,
                () ->
                        tx(
                                () ->
                                        trade.handle(
                                                "StockOrderVoid",
                                                n(stock, "id"),
                                                map(),
                                                Map.of(),
                                                f.actor())));
        tx(() -> trade.reverse(f.order(), f.actor()));
        var reversed = trade.order(f.order(), f.actor());
        assertEquals(first.get("auditedAt"), reversed.get("auditedAt"));
        assertNotNull(reversed.get("pendingSince"));
        assertNotNull(reversed.get("reversedAt"));
        assertEquals(2, n(db.find("inv_stock_orders", n(stock, "id")), "status"));
        tx(() -> trade.audit(f.order(), map("status", 2), f.actor()));
        var history = db.where("inv_order_audits", "orderId", f.order());
        assertEquals(
                List.of("audit", "reverse", "audit"),
                history.stream().map(e -> s(e, "action")).toList());
        assertEquals(
                java.time.OffsetDateTime.parse(first.get("auditedAt").toString()).toInstant(),
                java.time.OffsetDateTime.parse(history.get(0).get("occurredAt").toString())
                        .toInstant());
        assertEquals(2, db.where("inv_stock_orders", "sourceId", f.order()).size());
        assertThrows(
                BusinessException.class,
                () -> tx(() -> trade.audit(f.order(), map("status", 2), f.actor())));
        assertEquals(3, db.where("inv_order_audits", "orderId", f.order()).size());
        tx(() -> trade.settleOrder(map("orderId", f.order(), "amount", 1), f.actor()));
        assertThrows(BusinessException.class, () -> tx(() -> trade.reverse(f.order(), f.actor())));
        assertEquals(3, db.where("inv_order_audits", "orderId", f.order()).size());
    }

    @Test
    void insufficientStockAndEventWriteFailureRollBackAllSideEffects() {
        var f = fixture();
        tx(() -> trade.audit(f.order(), map("status", 2), f.actor()));
        db.adjustStock(f.spec(), dec(-10));
        assertThrows(BusinessException.class, () -> tx(() -> trade.reverse(f.order(), f.actor())));
        assertEquals(2, n(db.find("inv_orders", f.order()), "status"));
        assertEquals(1, n(db.where("inv_stock_orders", "sourceId", f.order()).get(0), "status"));
        assertEquals(1, db.where("inv_order_audits", "orderId", f.order()).size());
        assertEquals(0, d(db.find("inv_product_specs", f.spec()), "stock").signum());

        var failed = fixture();
        // Force the last transactional write to fail, after inventory and status changed.
        org.mockito.Mockito.doThrow(new IllegalStateException("simulated event storage failure"))
                .when(db)
                .insert(
                        org.mockito.ArgumentMatchers.eq("inv_order_audits"),
                        org.mockito.ArgumentMatchers.anyMap());
        try {
            assertThrows(
                    RuntimeException.class,
                    () -> tx(() -> trade.audit(failed.order(), map("status", 2), failed.actor())));
        } finally {
            org.mockito.Mockito.reset(db);
        }
        assertEquals(1, n(db.find("inv_orders", failed.order()), "status"));
        assertNull(db.find("inv_orders", failed.order()).get("auditedAt"));
        assertTrue(db.where("inv_order_audits", "orderId", failed.order()).isEmpty());
        assertTrue(db.where("inv_stock_orders", "sourceId", failed.order()).isEmpty());
        assertEquals(0, d(db.find("inv_product_specs", failed.spec()), "stock").signum());
    }

    @Test
    void concurrentAuditProducesOneEventAndScopeDoesNotExpand() throws Exception {
        var f = fixture();
        var pool = Executors.newFixedThreadPool(2);
        var gate = new CountDownLatch(1);
        Callable<Boolean> attempt =
                () -> {
                    gate.await();
                    try {
                        tx(() -> trade.audit(f.order(), map("status", 2), f.actor()));
                        return true;
                    } catch (BusinessException e) {
                        return false;
                    }
                };
        try {
            var a = pool.submit(attempt);
            var b = pool.submit(attempt);
            gate.countDown();
            assertNotEquals(a.get(10, TimeUnit.SECONDS), b.get(10, TimeUnit.SECONDS));
        } finally {
            pool.shutdownNow();
        }
        assertEquals(1, db.where("inv_order_audits", "orderId", f.order()).size());
        assertEquals(0, d(db.find("inv_product_specs", f.spec()), "stock").compareTo(dec(10)));
        var reader =
                new Actor(
                        f.actor().id(),
                        f.actor().user(),
                        List.of(),
                        Set.of("inv:purchase:list"),
                        false,
                        4);
        var detail = trade.order(f.order(), reader);
        assertEquals("restricted", ((Map<?, ?>) detail.get("downstream")).get("visibility"));
        assertTrue(trade.orders(reader, "purchase", "auditable").isEmpty());
        assertTrue(
                trade.orders(reader, "purchase", "created").stream()
                        .allMatch(o -> n(o, "createdBy") == reader.id()));
        assertThrows(BusinessException.class, () -> trade.orders(reader, "purchase", "invalid"));
    }
}
