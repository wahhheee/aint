// Package netx 提供模型与认证请求共用的网络配置。
package netx

import "net/http"

// ProxyFromEnvironment 优先使用代理环境变量，Windows 缺省时使用系统手动代理。
var ProxyFromEnvironment = http.ProxyFromEnvironment
