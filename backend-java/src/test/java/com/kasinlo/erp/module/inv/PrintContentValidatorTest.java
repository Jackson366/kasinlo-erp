package com.kasinlo.erp.module.inv;

import com.kasinlo.erp.common.BusinessException;
import java.awt.image.BufferedImage;
import java.io.ByteArrayOutputStream;
import java.util.Base64;
import javax.imageio.ImageIO;
import org.junit.jupiter.api.Test;
import static org.junit.jupiter.api.Assertions.*;

class PrintContentValidatorTest {
    private String png(boolean transparent) throws Exception {
        var image = new BufferedImage(2, 2, BufferedImage.TYPE_INT_ARGB);
        image.setRGB(0, 0, 0xffff0000);
        if (!transparent) {
            for (int y = 0; y < 2; y++) {
                for (int x = 0; x < 2; x++) image.setRGB(x, y, 0xffff0000);
            }
        }
        var bytes = new ByteArrayOutputStream();
        ImageIO.write(image, "png", bytes);
        return "{\"stamp\":{\"enabled\":true,\"image\":\"data:image/png;base64," +
            Base64.getEncoder().encodeToString(bytes.toByteArray()) + "\"}}";
    }

    @Test
    void acceptsLegacyAndTransparentPng() throws Exception {
        for (String content : new String[]{"", "{}", "{\"footer\":\"旧页脚\"}", png(true)}) {
            assertDoesNotThrow(() -> PrintContentValidator.validate(content));
        }
    }

    @Test
    void rejectsBadConfigAndOpaqueOrRemoteImages() throws Exception {
        for (String content : new String[]{"null", "[]", "broken", "a".repeat(512001),
            "{\"stamp\":{\"enabled\":true}}", "{\"stamp\":{\"image\":\"https://example.com/seal.png\"}}",
            "{\"stamp\":{\"image\":\"data:image/png;base64,YQ==\"}}", png(false)}) {
            assertThrows(BusinessException.class, () -> PrintContentValidator.validate(content));
        }
    }
}
