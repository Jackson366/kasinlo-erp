package com.kasinlo.erp.infrastructure.inventory;

import com.kasinlo.erp.app.inventory.InventoryQueryGateway;
import com.kasinlo.erp.client.InventoryQuery.Row;
import org.apache.ibatis.annotations.Mapper;
import org.apache.ibatis.annotations.Param;

import java.util.List;

@Mapper
public interface InventoryMapper extends InventoryQueryGateway {
    @Override
    long count(@Param("pattern") String pattern);

    @Override
    List<Row> list(@Param("pattern") String pattern, @Param("limit") int limit, @Param("offset") long offset);
}
