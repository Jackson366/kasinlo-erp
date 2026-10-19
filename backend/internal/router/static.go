package router

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// staticBackendPrefixes 是「即使前端目录里恰好有同名文件，也必须走后端」的路径前缀。
//
// 不这样挡的话，前端目录里放一个 api.html 就能把 /api 的 404 变成一个 200 页面，
// 接口调试时看到「返回 HTML」会非常难查。
var staticBackendPrefixes = []string{"/api", "/swagger", "/health", "/metrics"}

// Static 返回前端静态产物的处理器，用于 engine.NoRoute。
//
// 返回 nil 表示「不托管」，由调用方回落到原来的 JSON 404：
//   - 没配置目录 / 目录不存在
//   - 目录里没有 index.html（多半是前端还没构建或放错位置）
//
// 这里刻意不因为「前端没放对」而让服务起不来：后端的可用性与前端产物
// 的解耦很关键，现场升级时前端先行/后行都能接受。
//
// 路由策略：
//   - 命中真实文件 → 直出（/assets/ 下的带 hash 文件给长缓存）
//   - 其余 GET/HEAD → 回退 index.html（SPA 深链刷新）
//   - 非 GET/HEAD、或 /api 等后端前缀 → 交回 JSON 404
func Static(dir string) gin.HandlerFunc {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return nil
	}
	index := filepath.Join(abs, "index.html")
	if st, err := os.Stat(index); err != nil || st.IsDir() {
		return nil
	}

	return func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "接口不存在: " + c.Request.URL.Path})
			return
		}
		p := c.Request.URL.Path
		for _, prefix := range staticBackendPrefixes {
			if p == prefix || strings.HasPrefix(p, prefix+"/") {
				c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "接口不存在: " + p})
				return
			}
		}

		clean := strings.TrimPrefix(filepath.Clean(p), string(filepath.Separator))
		if clean == "" || clean == "." {
			serveIndex(c, index)
			return
		}
		full := filepath.Join(abs, clean)
		// 防目录穿越：filepath.Join 已经吃掉 ..，这里再确认结果仍在根目录下。
		if full != abs && !strings.HasPrefix(full, abs+string(filepath.Separator)) {
			serveIndex(c, index)
			return
		}
		if st, err := os.Stat(full); err == nil && !st.IsDir() {
			if strings.HasPrefix(clean, "assets"+string(filepath.Separator)) {
				// vite 产物文件名带内容 hash，内容变了文件名就变，可以放心长缓存
				c.Header("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				c.Header("Cache-Control", "public, max-age=3600")
			}
			c.File(full)
			return
		}
		serveIndex(c, index)
	}
}

// serveIndex 输出入口页。不缓存：发版换掉 dist 后，用户刷新必须立刻拿到新 index，
// 否则旧 index 引用的旧 assets 已被清理，页面直接白屏。
func serveIndex(c *gin.Context, index string) {
	c.Header("Cache-Control", "no-cache")
	c.File(index)
}
