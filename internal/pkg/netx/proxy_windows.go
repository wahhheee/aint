package netx

import (
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"

	"golang.org/x/net/http/httpproxy"
	"golang.org/x/sys/windows/registry"
)

func init() {
	cfg := httpproxy.FromEnvironment()
	if cfg.HTTPProxy != "" || cfg.HTTPSProxy != "" {
		return
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return
	}
	defer func() { _ = key.Close() }()
	enabled, _, err := key.GetIntegerValue("ProxyEnable")
	if err != nil || enabled == 0 {
		return
	}
	server, _, err := key.GetStringValue("ProxyServer")
	if err != nil || server == "" {
		return
	}
	bypass, _, _ := key.GetStringValue("ProxyOverride")
	ProxyFromEnvironment = windowsProxy(cfg, server, bypass)
}

func windowsProxy(cfg *httpproxy.Config, server, bypass string) func(*http.Request) (*url.URL, error) {
	for entry := range strings.SplitSeq(server, ";") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		protocol, address, specific := strings.Cut(entry, "=")
		if !specific {
			cfg.HTTPProxy, cfg.HTTPSProxy = protocol, protocol
			continue
		}
		switch strings.ToLower(protocol) {
		case "http":
			cfg.HTTPProxy = address
		case "https":
			cfg.HTTPSProxy = address
		}
	}
	proxy := cfg.ProxyFunc()
	rules := strings.Split(strings.ToLower(bypass), ";")
	return func(req *http.Request) (*url.URL, error) {
		host := strings.ToLower(req.URL.Hostname())
		port := req.URL.Port()
		if port == "" {
			port = "80"
			if req.URL.Scheme == "https" {
				port = "443"
			}
		}
		for _, rule := range rules {
			rule = strings.TrimSpace(rule)
			if rule == "<local>" && !strings.ContainsAny(host, ".:") {
				return nil, nil
			}
			if matches, _ := path.Match(rule, host); matches {
				return nil, nil
			}
			if matches, _ := path.Match(rule, net.JoinHostPort(host, port)); matches {
				return nil, nil
			}
		}
		return proxy(req.URL)
	}
}
