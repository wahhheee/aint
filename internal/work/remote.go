package work

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/wahhheee/aint/internal/pkg/errs"
	"github.com/wahhheee/aint/internal/pkg/jsonx"
)

// RemoteRequest 指定链接来源与下载预算；显式 ref 与 subdir 用于消除 GitHub tree 链接的歧义。
type RemoteRequest struct {
	Source         string `json:"source"`
	URL            string `json:"url"`
	Ref            string `json:"ref"`
	Subdir         string `json:"subdir"`
	Path           string `json:"path"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	MaxDownloadMiB int    `json:"max_download_mib"`
}

type remoteSource struct {
	client *http.Client
	token  func() string
}

func newRemoteSource() *remoteSource {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	// 直接连接经过验证的地址，避免 DNS 重绑定或重定向访问本机和内网服务。
	tr.Proxy = nil
	tr.DialContext = publicDial
	return &remoteSource{client: &http.Client{Transport: tr, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errs.Invalid("下载链接重定向次数过多")
		}
		if via[0].URL.Hostname() == "api.github.com" || via[0].URL.Hostname() == "codeload.github.com" {
			if req.URL.Hostname() != via[0].URL.Hostname() {
				return errs.Invalid("GitHub 请求不能跳转到其他站点")
			}
		}
		return remoteURL(req.URL)
	}}}
}

func publicDial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, errs.Invalid("下载地址没有可用的 IP")
	}
	for _, ip := range ips {
		if !publicIP(ip) {
			return nil, errs.Invalid("链接导入只允许公网地址")
		}
	}
	var last error
	for _, ip := range ips {
		conn, err := (&net.Dialer{Timeout: 15 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		last = err
	}
	return nil, last
}

func publicIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	if ip.Is6() && !netip.MustParsePrefix("2000::/3").Contains(ip) {
		return false
	}
	for _, p := range []string{"100.64.0.0/10", "0.0.0.0/8", "192.0.0.0/24", "198.18.0.0/15", "240.0.0.0/4", "192.0.2.0/24", "198.51.100.0/24", "203.0.113.0/24", "2001::/23", "2001:db8::/32", "2002::/16"} {
		if netip.MustParsePrefix(p).Contains(ip) {
			return false
		}
	}
	return true
}

func remoteURL(u *url.URL) error {
	if u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return errs.Invalid("请填写不含账号密码或片段的 HTTPS 链接")
	}
	if ip, err := netip.ParseAddr(u.Hostname()); err == nil && !publicIP(ip) {
		return errs.Invalid("链接导入只允许公网地址")
	}
	return nil
}

// SetGitHubToken 设置读取 GitHub 令牌的回调。令牌只发往 api.github.com，不写入作品来源。
func (im *Importer) SetGitHubToken(token func() string) { im.remote.token = token }

func (rs *remoteSource) get(ctx context.Context, rawURL, etag string) (*http.Response, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, errs.Invalid("链接格式不正确")
	}
	if err := remoteURL(u); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "AINT")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if u.Hostname() == "api.github.com" {
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
		if rs.token != nil {
			if token := rs.token(); token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
		}
	}
	resp, err := rs.client.Do(req)
	if err != nil {
		return nil, errs.Invalid("下载失败，请检查链接或网络连接")
	}
	return resp, nil
}

var errRemoteNotFound = errors.New("远程资源不存在")

func remoteStatus(resp *http.Response) error {
	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusNotFound:
		return errRemoteNotFound
	case http.StatusForbidden, http.StatusTooManyRequests:
		return errs.Invalid("远程服务拒绝请求或已达到访问频率限制，请稍后重试；GitHub 可在设置中填写令牌")
	default:
		return errs.Invalid("远程服务返回 HTTP %d，请稍后重试", resp.StatusCode)
	}
}

func (rs *remoteSource) githubJSON(ctx context.Context, endpoint string, out any) error {
	resp, err := rs.get(ctx, "https://api.github.com/repos/"+endpoint, "")
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	// GitHub 对不存在的 ref 返回 422；仓库或资源不存在时返回 404。
	if resp.StatusCode == http.StatusUnprocessableEntity && strings.Contains(endpoint, "/commits/") {
		var detail struct {
			Message string `json:"message"`
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 8192))
		if err == nil && jsonx.Unmarshal(body, &detail) == nil && strings.HasPrefix(detail.Message, "No commit found for SHA:") {
			return errRemoteNotFound
		}
	}
	if err := remoteStatus(resp); err != nil {
		return err
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if err != nil {
		return err
	}
	if len(b) > 2<<20 {
		return errs.Invalid("GitHub 返回的元数据过大")
	}
	if err := jsonx.Unmarshal(b, out); err != nil {
		return errs.Invalid("GitHub 返回的元数据格式不正确")
	}
	return nil
}

var repoPart = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
var commitSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)

func (rs *remoteSource) commit(ctx context.Context, repo, ref string) (string, error) {
	var v struct {
		SHA string `json:"sha"`
	}
	if err := rs.githubJSON(ctx, repo+"/commits/"+url.PathEscape(ref), &v); err != nil {
		return "", err
	}
	if !commitSHA.MatchString(v.SHA) {
		return "", errs.Invalid("GitHub 没有返回有效的提交 SHA")
	}
	return v.SHA, nil
}

func (rs *remoteSource) resolve(ctx context.Context, req RemoteRequest) (Source, error) {
	u, err := url.Parse(req.URL)
	if err != nil {
		return Source{}, errs.Invalid("GitHub 链接格式不正确")
	}
	if err := remoteURL(u); err != nil {
		return Source{}, err
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if u.Host != "github.com" || u.RawQuery != "" || len(parts) < 2 || !repoPart.MatchString(parts[0]) || !repoPart.MatchString(parts[1]) || parts[0] == "." || parts[0] == ".." || parts[1] == "." || parts[1] == ".." {
		return Source{}, errs.Invalid("请填写 github.com 的公开仓库或 tree 链接")
	}
	src := Source{Type: "github", Repo: parts[0] + "/" + strings.TrimSuffix(parts[1], ".git")}
	if req.Subdir != "" {
		src.Subdir, err = normPath(req.Subdir)
		if err != nil {
			return Source{}, err
		}
	}
	if len(parts) > 2 && (len(parts) < 4 || parts[2] != "tree") {
		return Source{}, errs.Invalid("请使用仓库根或 tree 链接")
	}
	switch {
	case req.Ref != "":
		src.Ref = req.Ref
	case len(parts) == 2:
		var repo struct {
			DefaultBranch string `json:"default_branch"`
		}
		if err := rs.githubJSON(ctx, src.Repo, &repo); err != nil {
			return Source{}, err
		}
		if repo.DefaultBranch == "" {
			return Source{}, errs.Invalid("仓库没有默认分支")
		}
		src.Ref = repo.DefaultBranch
	default:
		rest := parts[3:]
		for n := len(rest); n > 0 && n > len(rest)-5; n-- {
			ref := strings.Join(rest[:n], "/")
			sha, err := rs.commit(ctx, src.Repo, ref)
			if errors.Is(err, errRemoteNotFound) {
				continue
			}
			if err != nil {
				return Source{}, err
			}
			src.Ref, src.Commit = ref, sha
			if req.Subdir == "" {
				src.Subdir = strings.Join(rest[n:], "/")
			}
			if src.Subdir != "" {
				src.Subdir, err = normPath(src.Subdir)
				if err != nil {
					return Source{}, err
				}
			}
			return src, nil
		}
		return Source{}, errs.Invalid("无法识别分支或子目录，请在高级选项中明确填写 ref 与作品子目录")
	}
	src.Commit, err = rs.commit(ctx, src.Repo, src.Ref)
	return src, err
}

func (im *Importer) download(ctx context.Context, src Source, maxBytes int64, conditional bool) (*FileSet, Source, error) {
	address, etag := src.URL, ""
	if conditional {
		etag = src.ETag
	}
	if src.Type == "github" {
		address = "https://codeload.github.com/" + src.Repo + "/zip/" + src.Commit
	}
	resp, err := im.remote.get(ctx, address, etag)
	if err != nil {
		return nil, src, err
	}
	defer func() { _ = resp.Body.Close() }()
	if conditional && resp.StatusCode == http.StatusNotModified {
		return nil, src, nil
	}
	if err := remoteStatus(resp); err != nil {
		return nil, src, err
	}
	if resp.ContentLength > maxBytes {
		return nil, src, rejectf("下载文件超过大小上限")
	}
	f, err := os.CreateTemp(im.dir, "download-*.zip")
	if err != nil {
		return nil, src, err
	}
	defer func() { _ = f.Close(); _ = os.Remove(f.Name()) }() // 临时下载内容不持久保存。
	n, err := io.Copy(f, io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, src, errs.Invalid("下载中断，请稍后重试")
	}
	if n > maxBytes {
		return nil, src, rejectf("下载文件超过大小上限")
	}
	var subdir *string
	if src.Type == "github" {
		subdir = &src.Subdir
	}
	set, err := fromZip(f, n, im.lim, subdir)
	if err != nil {
		return nil, src, err
	}
	src.ETag = resp.Header.Get("ETag")
	return set, src, nil
}

// PreviewRemote 下载后复用两段式导入，校验通过也必须确认才写入作品库。
func (im *Importer) PreviewRemote(ctx context.Context, req RemoteRequest) (*ImportPreview, error) {
	req.URL = strings.TrimSpace(req.URL)
	seconds := req.TimeoutSeconds
	if seconds == 0 {
		seconds = 120
	}
	if seconds < 5 || seconds > 300 {
		return nil, errs.Invalid("下载时限须在 5–300 秒之间")
	}
	maxBytes := im.lim.UploadLimit()
	if req.MaxDownloadMiB != 0 {
		if req.MaxDownloadMiB < 1 || int64(req.MaxDownloadMiB) > maxBytes>>20 {
			return nil, errs.Invalid("下载大小上限超出允许范围")
		}
		maxBytes = int64(req.MaxDownloadMiB) << 20
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(seconds)*time.Second)
	defer cancel()
	src := Source{Type: req.Source, URL: strings.TrimSpace(req.URL)}
	var err error
	switch req.Source {
	case "github":
		src, err = im.remote.resolve(ctx, req)
	case "url":
	default:
		return nil, errs.Invalid("链接来源应为 github 或 url")
	}
	if errors.Is(err, errRemoteNotFound) {
		return nil, errs.Invalid("公开仓库或分支不存在")
	}
	if err != nil {
		return nil, err
	}
	set, src, err := im.download(ctx, src, maxBytes, false)
	if errors.Is(err, errRemoteNotFound) {
		return nil, errs.Invalid("下载链接不存在")
	}
	if err != nil {
		return nil, err
	}
	return im.preview(ctx, set, src)
}

// UpdateCheck 的预览只登记暂存；确认后才成为作品版本。
type UpdateCheck struct {
	Changed bool           `json:"changed"`
	Preview *ImportPreview `json:"preview,omitzero"`
}

func (im *Importer) CheckUpdate(ctx context.Context, workID, versionID string) (*UpdateCheck, error) {
	v, err := getVersion(ctx, im.works.db.Read(), versionID)
	if err != nil || v.WorkID != workID {
		return nil, errs.NotFound("作品版本不存在")
	}
	src := v.Source
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	switch src.Type {
	case "github":
		sha, err := im.remote.commit(ctx, src.Repo, src.Ref)
		if errors.Is(err, errRemoteNotFound) {
			return nil, errs.Invalid("原仓库或分支已不存在")
		}
		if err != nil {
			return nil, err
		}
		if sha == src.Commit {
			return &UpdateCheck{}, nil
		}
		src.Commit = sha
	case "url":
	default:
		return nil, errs.Invalid("此版本没有可检查更新的链接来源")
	}
	set, src, err := im.download(ctx, src, im.lim.UploadLimit(), true)
	if errors.Is(err, errRemoteNotFound) {
		return nil, errs.Invalid("原下载链接已不存在")
	}
	if err != nil {
		return nil, err
	}
	if set == nil {
		return &UpdateCheck{}, nil
	}
	model, _ := Compile(set)
	if model.Manifest.ID != workID {
		return nil, errs.Invalid("链接中的作品 ID 与当前作品不同，请单独导入")
	}
	p, err := im.preview(ctx, set, src)
	if err != nil {
		return nil, fmt.Errorf("检查作品更新失败: %w", err)
	}
	return &UpdateCheck{Changed: set.TreeHash() != v.TreeHash, Preview: p}, nil
}
