package ops

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// 上传接口的限长必须靠 MaxBytesReader 在**读取阶段**生效：
// 先 FormFile 再 LimitReader 的话，gin 已经把超过 32 MB 的部分落到了临时文件，
// 一个「声明很小、实际一直发」的客户端足以把临时目录写满。
// 这里钉住的是「超限错误能被认出来」——认不出来就会退化成
// 「读取上传文件失败」，运维看不出是包太大。
func TestIsBodyTooLargeDetectsMaxBytesError(t *testing.T) {
	rec := httptest.NewRecorder()
	body := http.MaxBytesReader(rec, io.NopCloser(bytes.NewReader(make([]byte, 64))), 16)
	if _, err := io.ReadAll(body); err == nil {
		t.Fatal("超限读取应返回错误")
	} else if !isBodyTooLarge(err) {
		t.Fatalf("超限错误应被识别为请求体过大，实际: %v", err)
	}
}

// 反过来：普通 IO 错误不能被当成超限，否则磁盘满/连接中断会被误报成「包太大」，
// 让人去改一个根本不存在的大小问题。
func TestIsBodyTooLargeIgnoresOtherErrors(t *testing.T) {
	if isBodyTooLarge(errors.New("connection reset by peer")) {
		t.Fatal("普通错误不应被识别为请求体过大")
	}
	if isBodyTooLarge(nil) {
		t.Fatal("nil 不应被识别为请求体过大")
	}
}

// multipart 的 boundary 与字段也要算在请求体里，所以得上界留出余量；
// 否则「刚好等于上限的包」会被 boundary 那几行挤出去。
func TestMaxMultipartSlackIsPositive(t *testing.T) {
	if maxMultipartSlack <= 0 {
		t.Fatal("必须为 multipart 边界预留余量")
	}
	if maxMultipartSlack > maxPackageBytes {
		t.Fatal("余量不该比包上限还大，否则形同虚设")
	}
}
