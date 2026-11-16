package com.kasinlo.erp.adapter;

import com.kasinlo.erp.common.error.CommonErrorType;
import com.kasinlo.erp.common.error.ErpException;
import com.kasinlo.erp.domain.auth.InventoryAccess;
import com.kasinlo.erp.domain.auth.InventoryAccessConstants;
import com.nimbusds.jose.JOSEException;
import com.nimbusds.jose.JWSAlgorithm;
import com.nimbusds.jose.crypto.MACVerifier;
import com.nimbusds.jwt.JWTClaimsSet;
import com.nimbusds.jwt.SignedJWT;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;

import java.nio.charset.StandardCharsets;
import java.text.ParseException;
import java.time.Clock;
import java.time.Duration;
import java.time.Instant;

@Component
public class PlatformInventoryAuthorization {
    private final InventoryAccess access;
    private final Clock clock;
    private final MACVerifier verifier;
    private final String issuer;
    private final String audience;

    public PlatformInventoryAuthorization(InventoryAccess access, Clock clock,
            @Value("${erp.platform.secret:}") String secret,
            @Value("${erp.platform.issuer:}") String issuer,
            @Value("${erp.platform.audience:}") String audience) throws JOSEException {
        this.access = access;
        this.clock = clock;
        this.issuer = issuer;
        this.audience = audience;
        if (!secret.isEmpty() && (issuer.isBlank() || audience.isBlank())) {
            throw new IllegalArgumentException(PlatformAuthorizationConstants.INVALID_SIGNING_CONFIGURATION_MESSAGE);
        }
        this.verifier = secret.isEmpty() ? null : new MACVerifier(secret.getBytes(StandardCharsets.UTF_8));
    }

    public void requireRead(String authorization) {
        if (verifier == null) {
            throw new ErpException(CommonErrorType.RESOURCE_NOT_FOUND);
        }
        if (authorization == null || !authorization.regionMatches(true, 0, "Bearer ", 0, 7)
                || authorization.substring(7).isBlank()) {
            throw new ErpException(CommonErrorType.LOGIN_REQUIRED);
        }
        long userId;
        long epoch;
        try {
            SignedJWT token = SignedJWT.parse(authorization.substring(7).strip());
            if (!JWSAlgorithm.HS256.equals(token.getHeader().getAlgorithm()) || !token.verify(verifier)) {
                throw invalidToken();
            }
            JWTClaimsSet claims = token.getJWTClaimsSet();
            validateClaims(claims);
            userId = Long.parseLong(claims.getSubject());
            Object value = claims.getClaim("epoch");
            if (userId <= 0 || !(value instanceof Long || value instanceof Integer)) {
                throw invalidToken();
            }
            epoch = ((Number) value).longValue();
            if (!PlatformAuthorizationConstants.INVENTORY_READ_SCOPE.equals(claims.getStringClaim("scope"))) {
                throw new ErpException(CommonErrorType.FORBIDDEN);
            }
        } catch (ParseException | JOSEException | NumberFormatException e) {
            throw invalidToken();
        }

        switch (access.check(userId, epoch)) {
            case ALLOWED -> { }
            case USER_MISSING -> throw new ErpException(CommonErrorType.USER_NOT_FOUND);
            case ACCOUNT_DISABLED -> throw new ErpException(CommonErrorType.ACCOUNT_DISABLED);
            case TOKEN_REVOKED -> throw invalidToken();
            case FORBIDDEN -> throw new ErpException(
                    CommonErrorType.PERMISSION_REQUIRED, InventoryAccessConstants.STOCK_LIST_PERMISSION);
        }
    }

    private void validateClaims(JWTClaimsSet claims) throws ParseException {
        if (!issuer.equals(claims.getIssuer()) || !claims.getAudience().contains(audience)
                || claims.getSubject() == null || claims.getIssueTime() == null || claims.getExpirationTime() == null
                || claims.getStringClaim("grant_id") == null || claims.getStringClaim("grant_id").isEmpty()) {
            throw invalidToken();
        }
        Instant now = clock.instant();
        Instant issued = claims.getIssueTime().toInstant();
        Instant expires = claims.getExpirationTime().toInstant();
        if (issued.isAfter(now) || !expires.isAfter(now) || !expires.isAfter(issued)
                || Duration.between(issued, expires).compareTo(Duration.ofMinutes(1)) > 0
                || (claims.getNotBeforeTime() != null && claims.getNotBeforeTime().toInstant().isAfter(now))) {
            throw invalidToken();
        }
    }

    private static ErpException invalidToken() {
        return new ErpException(CommonErrorType.INVALID_TOKEN);
    }
}
