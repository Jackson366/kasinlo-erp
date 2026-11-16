package com.kasinlo.erp.common.result;

import java.util.List;

public record PageResult<T>(List<T> list, long total, long page, int pageSize) {}
