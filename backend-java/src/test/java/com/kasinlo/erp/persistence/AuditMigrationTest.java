package com.kasinlo.erp.persistence;

import static org.junit.jupiter.api.Assertions.*;

import org.flywaydb.core.Flyway;
import org.junit.jupiter.api.Test;

import java.sql.DriverManager;

class AuditMigrationTest {
    @Test
    void v2OrdersKeepTheirStatusAndHaveNoInventedAuditData() throws Exception {
        String url =
                "jdbc:h2:mem:audit-upgrade;MODE=PostgreSQL;DATABASE_TO_LOWER=TRUE;DB_CLOSE_DELAY=-1";
        Flyway.configure().dataSource(url, "sa", "").target("2").load().migrate();
        try (var connection = DriverManager.getConnection(url, "sa", "");
                var statement = connection.createStatement()) {
            statement.executeUpdate(
                    "INSERT INTO inv_orders (order_no, biz_type, status) VALUES ('HISTORICAL',"
                        + " 'purchase', 2)");
        }
        Flyway.configure().dataSource(url, "sa", "").load().migrate();
        try (var connection = DriverManager.getConnection(url, "sa", "");
                var statement = connection.createStatement()) {
            var rows =
                    statement.executeQuery(
                            "SELECT status, auditor_id, audited_at, reversed_by, reversed_at,"
                                + " pending_since FROM inv_orders WHERE order_no = 'HISTORICAL'");
            assertTrue(rows.next());
            assertEquals(2, rows.getInt(1));
            for (int i = 2; i <= 6; i++) assertNull(rows.getObject(i));
            var count = statement.executeQuery("SELECT COUNT(*) FROM inv_order_audits");
            assertTrue(count.next());
            assertEquals(0, count.getInt(1));
        }
    }
}
