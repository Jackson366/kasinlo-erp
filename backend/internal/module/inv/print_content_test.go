package inv

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func TestPrintStampValidation(t *testing.T) {
	im := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	im.Set(0, 0, color.NRGBA{R: 255, A: 255})
	var data bytes.Buffer
	if err := png.Encode(&data, im); err != nil {
		t.Fatal(err)
	}
	encoded := "data:image/png;base64," + base64.StdEncoding.EncodeToString(data.Bytes())
	valid, _ := json.Marshal(map[string]any{"stamp": map[string]any{"enabled": true, "image": encoded}})
	for _, content := range []string{"", "{}", `{"footer":"旧页脚"}`, string(valid)} {
		if err := validatePrintContent(content); err != nil {
			t.Fatalf("valid content rejected: %v", err)
		}
	}
	for _, content := range []string{"null", "[]", "broken", strings.Repeat("a", 512001), `{"stamp":{"enabled":true}}`, `{"stamp":{"image":"https://example.com/seal.png"}}`, `{"stamp":{"image":"data:image/png;base64,YQ=="}}`} {
		if err := validatePrintContent(content); err == nil {
			t.Fatal("invalid content accepted")
		}
	}
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			im.Set(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	data.Reset()
	if err := png.Encode(&data, im); err != nil {
		t.Fatal(err)
	}
	opaque, _ := json.Marshal(map[string]any{"stamp": map[string]any{"image": "data:image/png;base64," + base64.StdEncoding.EncodeToString(data.Bytes())}})
	if err := validatePrintContent(string(opaque)); err == nil {
		t.Fatal("opaque background accepted")
	}
}
