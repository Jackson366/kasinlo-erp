package com.kasinlo.erp.start;

import org.mybatis.spring.annotation.MapperScan;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;

import java.time.Clock;
import java.time.ZoneId;

@SpringBootApplication(scanBasePackages = "com.kasinlo.erp")
@MapperScan(basePackages = "com.kasinlo.erp.infrastructure", annotationClass = org.apache.ibatis.annotations.Mapper.class)
public class ErpApplication {
    public static void main(String[] args) {
        SpringApplication.run(ErpApplication.class, args);
    }

    @Bean
    Clock erpClock(@Value("${erp.time-zone:Asia/Shanghai}") String timeZone) {
        return Clock.system(ZoneId.of(timeZone));
    }
}
