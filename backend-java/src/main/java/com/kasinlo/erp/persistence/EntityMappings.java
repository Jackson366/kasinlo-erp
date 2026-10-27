package com.kasinlo.erp.persistence;

import com.baomidou.mybatisplus.core.mapper.BaseMapper;
import com.kasinlo.erp.persistence.entity.*;
import com.kasinlo.erp.persistence.mapper.*;
import java.util.Map;

/** Compile-time allowlist. ORM identifiers never come from HTTP input. */
public final class EntityMappings {
    public record Mapping(Class<?> entityType, Class<? extends BaseMapper<?>> mapperType) {
    }

    private static final Map<String, Mapping> TABLES = Map.ofEntries(Map.entry("sys_schema_versions",
        new Mapping(SysSchemaVersions.class, SysSchemaVersionsMapper.class)), Map.entry("sys_roles", new Mapping(SysRoles.class,
        SysRolesMapper.class)), Map.entry("sys_role_menus", new Mapping(SysRoleMenus.class, SysRoleMenusMapper.class)),
        Map.entry("sys_menus", new Mapping(SysMenus.class, SysMenusMapper.class)), Map.entry("sys_users",
        new Mapping(SysUsers.class, SysUsersMapper.class)), Map.entry("sys_user_roles", new Mapping(SysUserRoles.class,
        SysUserRolesMapper.class)), Map.entry("inv_customers", new Mapping(InvCustomers.class, InvCustomersMapper.class)),
        Map.entry("inv_suppliers", new Mapping(InvSuppliers.class, InvSuppliersMapper.class)), Map.entry("inv_categories",
        new Mapping(InvCategories.class, InvCategoriesMapper.class)), Map.entry("inv_products", new Mapping(InvProducts.class,
        InvProductsMapper.class)), Map.entry("inv_product_specs", new Mapping(InvProductSpecs.class, InvProductSpecsMapper.class)),
        Map.entry("inv_product_units", new Mapping(InvProductUnits.class, InvProductUnitsMapper.class)),
        Map.entry("inv_orders", new Mapping(InvOrders.class, InvOrdersMapper.class)),
        Map.entry("inv_order_audits", new Mapping(InvOrderAudits.class, InvOrderAuditsMapper.class)), Map.entry("inv_order_items",
        new Mapping(InvOrderItems.class, InvOrderItemsMapper.class)), Map.entry("inv_settlements", new Mapping(InvSettlements.class,
        InvSettlementsMapper.class)), Map.entry("inv_stock_orders", new Mapping(InvStockOrders.class,
        InvStockOrdersMapper.class)), Map.entry("inv_stock_items", new Mapping(InvStockItems.class, InvStockItemsMapper.class)),
        Map.entry("inv_print_templates", new Mapping(InvPrintTemplates.class, InvPrintTemplatesMapper.class)),
        Map.entry("inv_contracts", new Mapping(InvContracts.class, InvContractsMapper.class)), Map.entry("inv_contract_plans",
        new Mapping(InvContractPlans.class, InvContractPlansMapper.class)), Map.entry("inv_attachments",
        new Mapping(InvAttachments.class, InvAttachmentsMapper.class)), Map.entry("sys_operation_logs",
        new Mapping(SysOperationLogs.class, SysOperationLogsMapper.class)), Map.entry("sys_login_logs",
        new Mapping(SysLoginLogs.class, SysLoginLogsMapper.class)), Map.entry("sys_upgrade_records", new Mapping(SysUpgradeRecords.class,
        SysUpgradeRecordsMapper.class)), Map.entry("sys_idempotencies", new Mapping(SysIdempotencies.class,
        SysIdempotenciesMapper.class)), Map.entry("sys_depts", new Mapping(SysDepts.class, SysDeptsMapper.class)));

    private EntityMappings() {
    }

    public static Mapping require(String table) {
        Mapping mapping = TABLES.get(table);
        if (mapping == null) throw new IllegalArgumentException("Unknown table");
        return mapping;
    }
}
