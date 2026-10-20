package server

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// csp 是页面的内容安全策略（《02-技术方案》19）：作品内容不执行，面板 HTML 另经前端净化。
const csp = "default-src 'self'; img-src 'self' data: blob:; media-src 'self' blob:; font-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; frame-src 'none'; object-src 'none'"

// staticHandler 提供前端静态资源：
//   - /assets/*（Vite 带哈希的产物）长期缓存；index.html 不缓存；
//   - 其他非 /api 路径回退到 index.html（SPA 前端路由）；
//   - 前端尚未构建时显示说明页，告诉开发者下一步怎么做。
func (s *Server) staticHandler() http.Handler {
	web := s.d.Web
	if _, err := fs.Stat(web, "index.html"); err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(placeholderPage))
		})
	}
	files := http.FileServerFS(web)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(web, p); err != nil {
			p = "index.html" // SPA 回退
		}
		if p == "index.html" {
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Content-Security-Policy", csp)
			http.ServeFileFS(w, r, web, "index.html")
			return
		}
		if strings.HasPrefix(p, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		files.ServeHTTP(w, r)
	})
}

const placeholderPage = `<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><title>演义 · 前端未构建</title>
<style>
body{font-family:-apple-system,"Segoe UI","PingFang SC","Microsoft YaHei",sans-serif;background:#f7f5f0;color:#1f1e1c;
max-width:40em;margin:12vh auto;padding:0 1.5em;line-height:1.8}
h1{font-family:"Noto Serif SC","Songti SC",serif;font-weight:600;letter-spacing:.1em}
code{background:#efe8dc;padding:.15em .4em;border-radius:4px}
.seal{display:inline-block;background:#b5452f;color:#fff;font-family:serif;padding:.1em .35em;border-radius:3px;margin-right:.4em}
</style></head><body>
<h1><span class="seal">演</span>演义</h1>
<p>后端已在运行，但这个可执行文件里没有内嵌前端页面。</p>
<p><b>开发时</b>：运行 <code>task dev</code>（或 <code>pnpm -C web dev</code>），然后访问 <code>http://localhost:5173</code>。</p>
<p><b>发布构建</b>：先 <code>task build:web</code> 再 <code>task build</code>，前端会被嵌入可执行文件。</p>
<p>接口自检：<a href="/api/v1/system/info">/api/v1/system/info</a></p>
</body></html>`
