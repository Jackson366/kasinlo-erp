package inv

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image/png"
	"strings"

	"nova-erp/internal/pkg/errcode"
)

// Images travel in the existing access-controlled template content, never public asset URLs.
func validatePrintContent(content string) error {
	bad := func(message string) error { return errcode.New(errcode.ErrBadRequest, message) }
	if len(content) > 512000 {
		return bad("版式配置不能超过 512000 字节")
	}
	if strings.TrimSpace(content) == "" {
		return nil
	}
	var cfg map[string]json.RawMessage
	if json.Unmarshal([]byte(content), &cfg) != nil || cfg == nil {
		return bad("打印模板配置必须为 JSON 对象")
	}
	if len(cfg["stamp"]) == 0 || string(cfg["stamp"]) == "null" {
		return nil
	}
	var stamp struct {
		Enabled bool   `json:"enabled"`
		Image   string `json:"image"`
	}
	if json.Unmarshal(cfg["stamp"], &stamp) != nil {
		return bad("盖章区域配置不正确")
	}
	if stamp.Image == "" {
		if stamp.Enabled {
			return bad("启用盖章区域时请选择印章图片")
		}
		return nil
	}
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(stamp.Image, prefix) {
		return bad("印章仅支持透明背景 PNG 图片")
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stamp.Image, prefix))
	if err != nil || len(data) > 256*1024 {
		return bad("印章图片无效或超过 256 KB")
	}
	info, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || info.Width > 2048 || info.Height > 2048 {
		return bad("印章图片无效或宽高超过 2048 px")
	}
	image, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return bad("印章 PNG 图片无法读取")
	}
	transparent, visible := false, false
	for y := 0; y < info.Height; y++ {
		for x := 0; x < info.Width; x++ {
			_, _, _, a := image.At(x, y).RGBA()
			transparent = transparent || a < 65535
			visible = visible || a > 0
		}
	}
	if !transparent || !visible {
		return bad("请使用有可见印章的透明背景 PNG 图片")
	}
	return nil
}
