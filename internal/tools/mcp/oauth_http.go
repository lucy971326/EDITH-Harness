package mcp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

type oauthDiscovery struct {
	resource, issuer, metadataURL, authURL, tokenURL, registrationURL string
	cimdSupported                                                     bool
}

func loopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback()
}

func safeOAuthURL(raw string, allowLoopback bool) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return errors.New("OAuth URL 无效")
	}
	if u.Scheme != "https" && !(allowLoopback && u.Scheme == "http" && loopbackHost(u.Hostname())) {
		return errors.New("OAuth 地址必须是 HTTPS 或本机回环地址")
	}
	return nil
}

func sameOrigin(first, second *url.URL) bool {
	return strings.EqualFold(first.Scheme, second.Scheme) && strings.EqualFold(first.Host, second.Host)
}

// 发现请求不能转向内网；DialContext 再次核对解析结果，阻断 DNS 重绑定。
func oauthHTTPClient(endpoint string) *http.Client {
	u, _ := url.Parse(endpoint)
	allowLoopback := loopbackHost(u.Hostname())
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		resolved, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		var dialErr error
		allowed := false
		for _, item := range resolved {
			ip, ok := netip.AddrFromSlice(item.IP)
			if !ok {
				continue
			}
			ip = ip.Unmap()
			if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || !ip.IsGlobalUnicast() {
				if !(allowLoopback && loopbackHost(host) && ip.IsLoopback()) {
					continue
				}
			}
			allowed = true
			conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
			dialErr = err
		}
		if allowed {
			return nil, dialErr
		}
		return nil, errors.New("OAuth 地址解析到不允许的网络")
	}
	return &http.Client{Transport: transport, Timeout: 20 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return errors.New("OAuth 重定向过多")
		}
		if !sameOrigin(via[0].URL, req.URL) {
			return errors.New("OAuth 重定向不能跨来源")
		}
		return safeOAuthURL(req.URL.String(), allowLoopback)
	}}
}

func discoverOAuth(ctx context.Context, spec serverSpec, challenge *http.Response, client *http.Client) (oauthDiscovery, error) {
	if err := safeOAuthURL(spec.URL, loopbackHost(mustParseURL(spec.URL).Hostname())); err != nil {
		return oauthDiscovery{}, err
	}
	type candidate struct{ url, resource string }
	var candidates []candidate
	if challenge != nil {
		parsed, err := oauthex.ParseWWWAuthenticate(challenge.Header.Values("WWW-Authenticate"))
		if err == nil {
			for _, item := range parsed {
				if item.Scheme == "bearer" && item.Params["resource_metadata"] != "" {
					candidates = append(candidates, candidate{item.Params["resource_metadata"], spec.URL})
				}
			}
		}
	}
	u := mustParseURL(spec.URL)
	u.RawQuery, u.Fragment = "", ""
	resource := u.String()
	u.Path = "/.well-known/oauth-protected-resource/" + strings.TrimLeft(u.Path, "/")
	candidates = append(candidates, candidate{u.String(), resource})
	root := *mustParseURL(spec.URL)
	root.Path, root.RawPath, root.RawQuery, root.Fragment = "", "", "", ""
	rootResource := root.String()
	root.Path = "/.well-known/oauth-protected-resource"
	candidates = append(candidates, candidate{root.String(), rootResource})
	for _, candidate := range candidates {
		if err := safeOAuthURL(candidate.url, loopbackHost(mustParseURL(spec.URL).Hostname())); err != nil {
			continue
		}
		prm, err := oauthex.GetProtectedResourceMetadata(ctx, candidate.url, candidate.resource, client)
		if err != nil || prm == nil {
			continue
		}
		if len(prm.AuthorizationServers) == 0 {
			return oauthDiscovery{}, errors.New("MCP 授权说明缺少授权服务")
		}
		issuer := prm.AuthorizationServers[0]
		if err := safeOAuthURL(issuer, loopbackHost(mustParseURL(spec.URL).Hostname())); err != nil {
			return oauthDiscovery{}, err
		}
		asm, err := auth.GetAuthServerMetadata(ctx, issuer, client)
		if err != nil {
			return oauthDiscovery{}, errors.New("授权服务元数据无效")
		}
		if asm == nil {
			return oauthDiscovery{}, errors.New("授权服务未提供元数据")
		}
		if !slices.Contains(asm.CodeChallengeMethodsSupported, "S256") {
			return oauthDiscovery{}, errors.New("授权服务不支持 PKCE S256")
		}
		for _, endpoint := range []string{asm.AuthorizationEndpoint, asm.TokenEndpoint, asm.RegistrationEndpoint} {
			if endpoint != "" {
				if err := safeOAuthURL(endpoint, loopbackHost(mustParseURL(spec.URL).Hostname())); err != nil {
					return oauthDiscovery{}, err
				}
			}
		}
		return oauthDiscovery{resource: prm.Resource, issuer: asm.Issuer, metadataURL: candidate.url,
			authURL: asm.AuthorizationEndpoint, tokenURL: asm.TokenEndpoint,
			registrationURL: asm.RegistrationEndpoint, cimdSupported: asm.ClientIDMetadataDocumentSupported}, nil
	}
	return oauthDiscovery{}, fmt.Errorf("MCP Server 未提供有效的受保护资源元数据")
}

func mustParseURL(raw string) *url.URL {
	u, _ := url.Parse(raw)
	if u == nil {
		return &url.URL{}
	}
	return u
}

// 无凭据探测只用于定位挑战中的 resource_metadata，不发送已保存的令牌。
func probeOAuth(ctx context.Context, spec serverSpec, client *http.Client) *http.Response {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, spec.URL, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden {
		return nil
	}
	return resp
}
