package com.kasinlo.erp.persistence.mapper;

import com.kasinlo.erp.persistence.MaintenanceSql;
import org.apache.ibatis.annotations.*;
import java.util.List;
import java.util.Map;

/** Physical operations only; ordinary CRUD uses typed BaseMapper interfaces. */
public interface MaintenanceMapper {
    @Select("SELECT 1")
    int ping();

    @SelectProvider(type = MaintenanceSql.class, method = "selectPhysical")
    List<Map<String, Object>> physicalRows(
    @Param("table")
    String table);

    @DeleteProvider(type = MaintenanceSql.class, method = "purge")
    int purge(
    @Param("table")
    String table);

    @InsertProvider(type = MaintenanceSql.class, method = "restore")
    int restore(
    @Param("table")
    String table,
    @Param("values")
    Map<String, Object> values);

    @UpdateProvider(type = MaintenanceSql.class, method = "resetH2")
    void resetH2(
    @Param("table")
    String table,
    @Param("next")
    long next);

    @Select("SELECT setval(pg_get_serial_sequence(#{table}, 'id'), #{next}, false)")
    long resetPostgres(
    @Param("table")
    String table,
    @Param("next")
    long next);
}
