package work

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func remoteResponse(code int, body string) *http.Response {
	return &http.Response{StatusCode: code, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body))}
}

func TestGitHubSubdirectoryAndUpdate(t *testing.T) {
	works, im := newTestImporter(t)
	sha := strings.Repeat("a", 40)
	archive := makeZip(t, zf{name: "repo/README.md", data: "仓库说明"}, zf{name: "repo/stories/demo/work.yaml", data: minimalManifest}, zf{name: "repo/stories/demo/prompts/narrator.md", data: "讲一个故事"})
	im.SetGitHubToken(func() string { return "secret-test-token" })
	apiCalls, downloads := 0, 0
	im.remote.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Host {
		case "api.github.com":
			apiCalls++
			if r.Header.Get("Authorization") != "Bearer secret-test-token" {
				t.Error("API 请求缺少令牌")
			}
			if r.URL.Path == "/repos/author/book/commits/feature/story" {
				return remoteResponse(200, `{"sha":"`+sha+`"}`), nil
			}
			return remoteResponse(422, `{"message":"No commit found for SHA: unknown"}`), nil
		case "codeload.github.com":
			downloads++
			if r.Header.Get("Authorization") != "" {
				t.Error("令牌泄漏到下载站点")
			}
			if !strings.HasSuffix(r.URL.Path, sha) {
				t.Error("下载没有固定到提交")
			}
			return remoteResponse(200, string(archive)), nil
		default:
			t.Fatalf("意外请求：%s", r.URL.Host)
			return nil, nil
		}
	})
	ctx := context.Background()
	p, err := im.PreviewRemote(ctx, RemoteRequest{Source: "github", URL: "https://github.com/author/book/tree/feature/story/stories/demo"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Report.HasErrors() {
		t.Fatal(p.Report)
	}
	res, err := im.Confirm(ctx, p.Task)
	if err != nil {
		t.Fatal(err)
	}
	set, err := works.FileSet(ctx, res.WorkID, res.VersionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Paths()) != 2 {
		t.Fatal("归档子目录选择错误", set.Paths())
	}
	updated, err := im.CheckUpdate(ctx, res.WorkID, res.VersionID)
	if err != nil || updated.Changed || downloads != 1 {
		t.Fatal("同提交不应下载", updated, err)
	}
	sha = strings.Repeat("b", 40)
	updated, err = im.CheckUpdate(ctx, res.WorkID, res.VersionID)
	if err != nil || updated.Changed || updated.Preview == nil {
		t.Fatal("同内容新提交应可确认来源", updated, err)
	}
	again, err := im.Confirm(ctx, updated.Preview.Task)
	if err != nil || again.Created || again.VersionID != res.VersionID {
		t.Fatal(again, err)
	}
	v, err := getVersion(ctx, works.db.Read(), res.VersionID)
	if err != nil || v.Source.Commit != sha || v.Source.Ref != "feature/story" {
		t.Fatal(v, err)
	}
	if apiCalls > 6 {
		t.Fatalf("ref 解析消耗过多请求：%d", apiCalls)
	}
}

func TestURLConditionalAndDownloadLimits(t *testing.T) {
	_, im := newTestImporter(t)
	archive := makeZip(t, zf{name: "work.yaml", data: minimalManifest}, zf{name: "prompts/narrator.md", data: "故事"})
	calls := 0
	im.remote.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Authorization") != "" {
			t.Error("URL 导入不得携带 GitHub 令牌")
		}
		if r.Header.Get("If-None-Match") == `"v1"` {
			return remoteResponse(304, ""), nil
		}
		res := remoteResponse(200, string(archive))
		res.Header.Set("ETag", `"v1"`)
		return res, nil
	})
	ctx := context.Background()
	p, err := im.PreviewRemote(ctx, RemoteRequest{Source: "url", URL: "https://example.com/story.zip"})
	if err != nil {
		t.Fatal(err)
	}
	res, err := im.Confirm(ctx, p.Task)
	if err != nil {
		t.Fatal(err)
	}
	update, err := im.CheckUpdate(ctx, res.WorkID, res.VersionID)
	if err != nil || update.Changed || update.Preview != nil || calls != 2 {
		t.Fatal(update, err)
	}
	for _, length := range []int64{-1, 2 << 20} {
		im.remote.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			r := remoteResponse(200, strings.Repeat("x", 2<<20))
			r.ContentLength = length
			return r, nil
		})
		if _, err := im.PreviewRemote(ctx, RemoteRequest{Source: "url", URL: "https://example.com/big.zip", MaxDownloadMiB: 1}); err == nil {
			t.Fatal("下载预算未执行")
		}
	}
	im.remote.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := im.PreviewRemote(canceled, RemoteRequest{Source: "url", URL: "https://example.com/work.zip"}); err == nil {
		t.Fatal("取消未生效")
	}
	if entries := stagedEntries(t, im); len(entries) != 0 {
		t.Fatal("下载残留", entries)
	}
}

func TestRemoteAddressPolicy(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "::1", "10.0.0.1", "172.16.0.2", "192.168.1.3", "169.254.169.254", "::ffff:127.0.0.1", "100.64.1.2", "fc00::1", "64:ff9b::a00:1", "2002:7f00:1::", "2001:db8::1"} {
		if publicIP(netip.MustParseAddr(raw)) {
			t.Errorf("允许了保留地址：%s", raw)
		}
	}
	for _, raw := range []string{"1.1.1.1", "8.8.8.8", "2606:4700:4700::1111"} {
		if !publicIP(netip.MustParseAddr(raw)) {
			t.Errorf("拒绝公网地址：%s", raw)
		}
	}
	rs := newRemoteSource()
	first, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com/file", nil)
	for _, raw := range []string{"http://example.com/a", "https://127.0.0.1/a", "https://user:pass@example.com/a", "https://example.com/a#fragment"} {
		u, _ := url.Parse(raw)
		if remoteURL(u) == nil {
			t.Error("非法 URL 被接受", raw)
		}
		next := &http.Request{URL: u}
		if rs.client.CheckRedirect(next, []*http.Request{first}) == nil {
			t.Error("非法重定向被接受", raw)
		}
	}
	github, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://api.github.com/repos/a/b", nil)
	if rs.client.CheckRedirect(first, []*http.Request{github}) == nil {
		t.Fatal("GitHub 跨站重定向未拒绝")
	}
	unsafe := makeZip(t, zf{name: "repo/../escape", data: "外部文件"}, zf{name: "repo/story/work.yaml", data: minimalManifest})
	subdir := "story"
	if _, err := fromZip(bytes.NewReader(unsafe), int64(len(unsafe)), DefaultLimits, &subdir); err == nil {
		t.Fatal("子目录外的危险路径也应拒绝")
	}
}
