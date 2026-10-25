package com.kasinlo.erp.persistence;

import com.baomidou.mybatisplus.core.conditions.query.QueryWrapper;
import com.kasinlo.erp.common.BusinessException;
import com.kasinlo.erp.persistence.entity.SysUsers;
import com.kasinlo.erp.persistence.mapper.SysUsersMapper;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.transaction.annotation.Transactional;

import java.math.BigDecimal;
import java.time.OffsetDateTime;
import java.util.Map;

import static com.kasinlo.erp.common.Values.*;
import static org.junit.jupiter.api.Assertions.*;

@SpringBootTest(properties = {
    "spring.datasource.url=jdbc:h2:mem:mybatis-tests;MODE=PostgreSQL;DATABASE_TO_LOWER=TRUE;DB_CLOSE_DELAY=-1",
    "erp.demo-data=false"
})
@Transactional
class MybatisPersistenceTest {
    @Autowired ErpRepository db;
    @Autowired SysUsersMapper users;

    @Test void baseMapperGeneratesIdentityAndRetainsExplicitZeroAndFalse() {
        SysUsers entity = new SysUsers();
        entity.setUsername("mapper-user");
        entity.setStatus(0L);
        assertEquals(1, users.insert(entity));
        assertNotNull(entity.getId());
        assertEquals(0L, users.selectById(entity.getId()).getStatus());

        long role = db.insert("sys_roles", map("name", "disabled", "code", "disabled", "status", 0, "isBuiltin", false));
        var saved = db.find("sys_roles", role);
        assertEquals(0, n(saved, "status"));
        assertEquals(false, saved.get("isBuiltin"));
    }

    @Test void logicalDeleteFiltersReadsAndRetainsPhysicalHistory() {
        long id = db.insert("inv_customers", map("name", "history", "code", "history"));
        db.delete("inv_customers", id);
        assertTrue(db.where("inv_customers", "id", id).isEmpty());
        assertThrows(BusinessException.class, () -> db.find("inv_customers", id));
        var row = db.physicalRows("inv_customers").stream().filter(r -> n(r, "id") == id).findFirst().orElseThrow();
        assertNotNull(row.get("deletedAt"));
        assertThrows(BusinessException.class, () -> db.update("inv_customers", id, map("name", "revived")));
    }

    @Test void explicitNullAndEmptyStringClearStoredFields() {
        long id = db.insert("inv_contracts", map("name", "clearable", "auditedAt", OffsetDateTime.now(), "auditor", "before"));
        assertNotNull(db.find("inv_contracts", id).get("auditedAt"));
        db.update("inv_contracts", id, map("auditedAt", null, "auditor", ""));
        var result = db.find("inv_contracts", id);
        assertNull(result.get("auditedAt"));
        assertEquals("", result.get("auditor"));
    }

    @Test void compositeAssociationRowsUseConditionalCrud() {
        db.insert("sys_user_roles", map("userId", 90001, "roleId", 90002));
        db.insert("sys_user_roles", map("userId", 90001, "roleId", 90003));
        assertEquals(2, db.where("sys_user_roles", "userId", 90001).size());
        db.deleteWhere("sys_user_roles", "userId", 90001);
        assertTrue(db.where("sys_user_roles", "userId", 90001).isEmpty());
    }

    @Test void stockMapperGuardsAgainstNegativeInventoryAndPreservesScale() {
        long id = db.insert("inv_product_specs", map("productId", 0, "spec", "stock", "stock", new BigDecimal("1.2345")));
        assertEquals(0, db.adjustStock(id, new BigDecimal("-2")));
        assertEquals(1, db.adjustStock(id, new BigDecimal("-0.1234")));
        assertEquals(0, d(db.find("inv_product_specs", id), "stock").compareTo(new BigDecimal("1.1111")));
    }

    @Test void requestKeysRoundTripThroughTypedMapper() {
        db.createRequest("orm-test", 42, "fingerprint");
        assertEquals("fingerprint", db.lockRequest("orm-test").get("fingerprint"));
        db.finishRequest("orm-test", "{\"code\":0}");
        assertEquals("{\"code\":0}", db.lockRequest("orm-test").get("response"));
    }

    @Test void snapshotRestorePreservesExplicitIdentityAndDeletedTimestamp() {
        long restoredId = 999999;
        db.restoreRow("inv_customers", map("id", restoredId, "name", "restored", "code", "restore", "deletedAt", OffsetDateTime.now()));
        assertTrue(db.where("inv_customers", "id", restoredId).isEmpty());
        assertTrue(db.physicalRows("inv_customers").stream().anyMatch(r -> n(r, "id") == restoredId && r.get("deletedAt") != null));
        assertThrows(IllegalArgumentException.class, () -> db.purge("inv_customers; DROP TABLE sys_users"));
        assertThrows(IllegalArgumentException.class, () -> db.where("inv_customers", "name) OR 1=1 --", "x"));
    }
}
