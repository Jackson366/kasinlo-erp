package com.kasinlo.erp.adapter;

import com.kasinlo.erp.client.InventoryQuery;
import com.kasinlo.erp.common.result.PageResult;
import org.springframework.http.CacheControl;
import org.springframework.http.HttpHeaders;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestHeader;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

@RestController
public class PlatformInventoryController {
    private final PlatformInventoryAuthorization authorization;
    private final InventoryQuery inventory;

    public PlatformInventoryController(PlatformInventoryAuthorization authorization, InventoryQuery inventory) {
        this.authorization = authorization;
        this.inventory = inventory;
    }

    @GetMapping("${erp.api-base-path:/api/v1}/internal/open-platform/inventory")
    public ResponseEntity<Result<PageResult<InventoryQuery.Row>>> list(
            @RequestHeader(name = HttpHeaders.AUTHORIZATION, required = false) String bearer,
            @RequestParam(required = false) String keyword,
            @RequestParam(required = false) String page,
            @RequestParam(required = false) String pageSize) {
        authorization.requireRead(bearer);
        var result = inventory.list(keyword, parse(page, 1), parse(pageSize, 10));
        return ResponseEntity.ok().cacheControl(CacheControl.noStore()).body(new Result<>(0, "success", result));
    }

    private static long parse(String value, long fallback) {
        if (value == null) {
            return fallback;
        }
        try {
            return Long.parseLong(value);
        } catch (NumberFormatException e) {
            return fallback;
        }
    }
}
