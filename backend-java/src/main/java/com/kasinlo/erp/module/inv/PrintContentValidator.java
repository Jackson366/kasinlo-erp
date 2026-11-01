package com.kasinlo.erp.module.inv;

import com.fasterxml.jackson.databind.ObjectMapper;
import javax.imageio.ImageIO;
import java.io.ByteArrayInputStream;
import java.nio.ByteBuffer;
import java.nio.charset.StandardCharsets;
import java.util.Base64;
import static com.kasinlo.erp.common.BusinessException.require;

final class PrintContentValidator {
    private static final ObjectMapper JSON = new ObjectMapper();
    private PrintContentValidator() {}

    static void validate(String content) {
        require(content.getBytes(StandardCharsets.UTF_8).length <= 512000, "版式配置不能超过 512000 字节");
        if (content.isBlank()) return;
        try {
            var config = JSON.readTree(content);
            require(config != null && config.isObject(), "打印模板配置必须为 JSON 对象");
            var stamp = config.get("stamp");
            if (stamp == null || stamp.isNull()) return;
            require(stamp.isObject(), "盖章区域配置不正确");
            String image = stamp.path("image").asText("");
            if (image.isEmpty()) {
                require(!stamp.path("enabled").asBoolean(false), "启用盖章区域时请选择印章图片");
                return;
            }
            String prefix = "data:image/png;base64,";
            require(image.startsWith(prefix), "印章仅支持透明背景 PNG 图片");
            byte[] data = Base64.getDecoder().decode(image.substring(prefix.length()));
            require(data.length >= 24 && data.length <= 256 * 1024, "印章图片无效或超过 256 KB");
            byte[] signature = {(byte)137, 80, 78, 71, 13, 10, 26, 10};
            for (int i = 0; i < signature.length; i++) require(data[i] == signature[i], "印章必须为 PNG 图片");
            var buffer = ByteBuffer.wrap(data);
            int width = buffer.getInt(16), height = buffer.getInt(20);
            require(width > 0 && height > 0 && width <= 2048 && height <= 2048, "印章图片宽高不能超过 2048 px");
            var decoded = ImageIO.read(new ByteArrayInputStream(data));
            require(decoded != null, "印章 PNG 图片无法读取");
            boolean transparent = false, visible = false;
            for (int y = 0; y < height; y++) {
                for (int x = 0; x < width; x++) {
                    int alpha = decoded.getRGB(x, y) >>> 24;
                    transparent |= alpha < 255;
                    visible |= alpha > 0;
                }
            }
            require(transparent && visible, "请使用有可见印章的透明背景 PNG 图片");
        } catch (com.kasinlo.erp.common.BusinessException error) {
            throw error;
        } catch (Exception error) {
            require(false, "打印配置或印章图片无法读取，请重新选择 PNG 后保存");
        }
    }
}
