package com.kasinlo.erp.infrastructure.auth;

import org.apache.ibatis.annotations.Mapper;
import org.apache.ibatis.annotations.Param;
import org.apache.ibatis.annotations.Select;

@Mapper
public interface InventoryAccessMapper {
    @Select("""
            SELECT u.status, COALESCE(u.token_epoch, 0) AS token_epoch,
                   EXISTS (
                       SELECT 1
                       FROM sys_user_roles ur
                       JOIN sys_roles r ON r.id = ur.role_id AND r.status = 1 AND r.deleted_at IS NULL
                       LEFT JOIN sys_role_menus rm ON rm.role_id = r.id
                       LEFT JOIN sys_menus m ON m.id = rm.menu_id AND m.status = 1 AND m.deleted_at IS NULL
                       WHERE ur.user_id = u.id AND (r.code = 'super_admin' OR m.permission = 'inv:stock:list')
                   ) AS allowed
            FROM sys_users u
            WHERE u.id = #{userId} AND u.deleted_at IS NULL
            """)
    UserAccess find(@Param("userId") long userId);

    record UserAccess(int status, long tokenEpoch, boolean allowed) {}
}
