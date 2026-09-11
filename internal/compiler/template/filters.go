package template

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"sync"

	"github.com/flosch/pongo2/v6"
	"gopkg.in/yaml.v3"

	"clash-sub-parser/internal/domain"
)

var registerOnce sync.Once

func init() {
	RegisterCustomFilters()
}

// RegisterCustomFilters registers custom filters in pongo2 global registry once.
func RegisterCustomFilters() {
	registerOnce.Do(func() {
		pongo2.SetAutoescape(false)

		_ = pongo2.RegisterFilter("base64", filterBase64)
		_ = pongo2.RegisterFilter("urlencode", filterURLEncode)
		_ = pongo2.RegisterFilter("json", filterJSON)
		_ = pongo2.RegisterFilter("yaml", filterYAML)
		_ = pongo2.RegisterFilter("regex_filter", filterRegex)
		_ = pongo2.RegisterFilter("surge_proxy", filterSurgeProxy)
		_ = pongo2.RegisterFilter("loon_proxy", filterLoonProxy)
		_ = pongo2.RegisterFilter("qx_server", filterQXServer)
		_ = pongo2.RegisterFilter("surge_rule", filterSurgeRule)
		_ = pongo2.RegisterFilter("loon_rule", filterLoonRule)
		_ = pongo2.RegisterFilter("qx_rule", filterQXRule)
		_ = pongo2.RegisterFilter("surge_group", filterSurgeGroup)
		_ = pongo2.RegisterFilter("loon_group", filterLoonGroup)
		_ = pongo2.RegisterFilter("qx_policy", filterQXPolicy)
		_ = pongo2.RegisterFilter("singbox_outbound", filterSingboxOutbound)
	})
}

func filterBase64(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	if in == nil || in.IsNil() {
		return pongo2.AsSafeValue(""), nil
	}
	var raw []byte
	switch v := in.Interface().(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		raw = []byte(fmt.Sprintf("%v", v))
	}
	return pongo2.AsSafeValue(base64.StdEncoding.EncodeToString(raw)), nil
}

func filterURLEncode(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	if in == nil || in.IsNil() {
		return pongo2.AsSafeValue(""), nil
	}
	s := in.String()
	return pongo2.AsSafeValue(url.QueryEscape(s)), nil
}

func filterJSON(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	if in == nil || in.IsNil() {
		return pongo2.AsSafeValue("null"), nil
	}
	indent := ""
	if param != nil && !param.IsNil() && param.String() != "" {
		indent = param.String()
	}

	var data []byte
	var err error
	if indent != "" {
		data, err = json.MarshalIndent(in.Interface(), "", indent)
	} else {
		data, err = json.Marshal(in.Interface())
	}
	if err != nil {
		return nil, &pongo2.Error{Sender: "filter:json", OrigError: err}
	}
	return pongo2.AsSafeValue(string(data)), nil
}

func filterYAML(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	if in == nil || in.IsNil() {
		return pongo2.AsSafeValue(""), nil
	}
	data, err := yaml.Marshal(in.Interface())
	if err != nil {
		return nil, &pongo2.Error{Sender: "filter:yaml", OrigError: err}
	}
	res := strings.TrimRight(string(data), "\n")
	indentSpaces := 0
	if param != nil && !param.IsNil() && param.Integer() > 0 {
		indentSpaces = param.Integer()
	}
	if indentSpaces > 0 {
		prefix := strings.Repeat(" ", indentSpaces)
		lines := strings.Split(res, "\n")
		for i, line := range lines {
			if strings.TrimSpace(line) != "" {
				lines[i] = prefix + line
			}
		}
		res = strings.Join(lines, "\n")
	}
	return pongo2.AsSafeValue(res), nil
}

func filterRegex(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	if in == nil || in.IsNil() {
		return pongo2.AsSafeValue([]any{}), nil
	}
	if param == nil || param.IsNil() || param.String() == "" {
		return in, nil
	}
	re, err := regexp.Compile(param.String())
	if err != nil {
		return nil, &pongo2.Error{Sender: "filter:regex_filter", OrigError: err}
	}

	val := reflect.ValueOf(in.Interface())
	if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
		return in, nil
	}

	out := make([]any, 0)
	for i := 0; i < val.Len(); i++ {
		item := val.Index(i).Interface()
		var name string
		switch v := item.(type) {
		case *domain.Node:
			if v != nil {
				name = v.Name
			}
		case domain.Node:
			name = v.Name
		case string:
			name = v
		case map[string]any:
			if n, ok := v["name"].(string); ok {
				name = n
			}
		default:
			name = fmt.Sprintf("%v", item)
		}

		if re.MatchString(name) {
			out = append(out, item)
		}
	}

	return pongo2.AsValue(out), nil
}

// NodeToSurge formats a node into a Surge proxy line.
func NodeToSurge(node *domain.Node) string {
	if node == nil {
		return ""
	}
	payload := node.NormalizedPayload
	if payload == nil {
		payload = map[string]any{}
	}

	proto := strings.ToLower(string(node.CanonicalProtocol()))
	name := strings.TrimSpace(node.Name)
	server := strings.TrimSpace(node.Server)
	port := node.Port

	switch proto {
	case "ss", "shadowsocks":
		cipher := getString(payload, "cipher", "aes-256-gcm")
		password := getString(payload, "password", "")
		return fmt.Sprintf("%s = ss, %s, %d, encrypt-method=%s, password=%s", name, server, port, cipher, password)
	case "vmess":
		uuid := getString(payload, "uuid", "")
		tls := getBool(payload, "tls", false)
		net := getString(payload, "network", "tcp")
		wsPath := ""
		if wsOpts, ok := payload["ws-opts"].(map[string]any); ok {
			wsPath = getString(wsOpts, "path", "")
		} else if wsOpts, ok := payload["ws_opts"].(map[string]any); ok {
			wsPath = getString(wsOpts, "path", "")
		}

		parts := []string{fmt.Sprintf("%s = vmess, %s, %d, username=%s", name, server, port, uuid)}
		if tls {
			parts = append(parts, "tls=true")
		}
		if net == "ws" {
			parts = append(parts, "ws=true")
			if wsPath != "" {
				parts = append(parts, fmt.Sprintf("ws-path=%s", wsPath))
			}
		}
		return strings.Join(parts, ", ")
	case "trojan":
		password := getString(payload, "password", "")
		sni := getString(payload, "sni", server)
		skipCert := getBool(payload, "skip-cert-verify", false)
		parts := []string{fmt.Sprintf("%s = trojan, %s, %d, password=%s", name, server, port, password)}
		if sni != "" && sni != server {
			parts = append(parts, fmt.Sprintf("sni=%s", sni))
		}
		if skipCert {
			parts = append(parts, "skip-cert-verify=true")
		}
		return strings.Join(parts, ", ")
	case "hysteria2", "hy2":
		password := getString(payload, "password", "")
		sni := getString(payload, "sni", server)
		skipCert := getBool(payload, "skip-cert-verify", false)
		parts := []string{fmt.Sprintf("%s = hysteria2, %s, %d, password=%s", name, server, port, password)}
		if sni != "" {
			parts = append(parts, fmt.Sprintf("sni=%s", sni))
		}
		if skipCert {
			parts = append(parts, "skip-cert-verify=true")
		}
		return strings.Join(parts, ", ")
	default:
		return ""
	}
}

func filterSurgeProxy(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	if in == nil || in.IsNil() {
		return pongo2.AsSafeValue(""), nil
	}
	if n, ok := in.Interface().(*domain.Node); ok {
		return pongo2.AsSafeValue(NodeToSurge(n)), nil
	}
	return pongo2.AsSafeValue(""), nil
}

// NodeToLoon formats a node into a Loon proxy line.
func NodeToLoon(node *domain.Node) string {
	if node == nil {
		return ""
	}
	payload := node.NormalizedPayload
	if payload == nil {
		payload = map[string]any{}
	}

	proto := strings.ToLower(string(node.CanonicalProtocol()))
	name := strings.TrimSpace(node.Name)
	server := strings.TrimSpace(node.Server)
	port := node.Port

	switch proto {
	case "ss", "shadowsocks":
		cipher := getString(payload, "cipher", "aes-256-gcm")
		password := getString(payload, "password", "")
		return fmt.Sprintf("%s = Shadowsocks,%s,%d,%s,\"%s\"", name, server, port, cipher, password)
	case "vmess":
		uuid := getString(payload, "uuid", "")
		tls := getBool(payload, "tls", false)
		net := getString(payload, "network", "tcp")
		wsPath := ""
		if wsOpts, ok := payload["ws-opts"].(map[string]any); ok {
			wsPath = getString(wsOpts, "path", "")
		} else if wsOpts, ok := payload["ws_opts"].(map[string]any); ok {
			wsPath = getString(wsOpts, "path", "")
		}

		parts := []string{fmt.Sprintf("%s = vmess,%s,%d,auto,\"%s\"", name, server, port, uuid)}
		if net == "ws" {
			parts = append(parts, "transport:ws")
			if wsPath != "" {
				parts = append(parts, fmt.Sprintf("path:\"%s\"", wsPath))
			}
		}
		if tls {
			parts = append(parts, "over-tls:true")
		}
		return strings.Join(parts, ",")
	case "trojan":
		password := getString(payload, "password", "")
		sni := getString(payload, "sni", server)
		parts := []string{fmt.Sprintf("%s = trojan,%s,%d,\"%s\"", name, server, port, password)}
		parts = append(parts, "over-tls:true")
		if sni != "" {
			parts = append(parts, fmt.Sprintf("tls-name:\"%s\"", sni))
		}
		return strings.Join(parts, ",")
	case "hysteria2", "hy2":
		password := getString(payload, "password", "")
		sni := getString(payload, "sni", server)
		parts := []string{fmt.Sprintf("%s = hysteria2,%s,%d,password=\"%s\"", name, server, port, password)}
		if sni != "" {
			parts = append(parts, fmt.Sprintf("sni=%s", sni))
		}
		return strings.Join(parts, ",")
	default:
		return ""
	}
}

func filterLoonProxy(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	if in == nil || in.IsNil() {
		return pongo2.AsSafeValue(""), nil
	}
	if n, ok := in.Interface().(*domain.Node); ok {
		return pongo2.AsSafeValue(NodeToLoon(n)), nil
	}
	return pongo2.AsSafeValue(""), nil
}

// NodeToQX formats a node into a Quantumult X server_local line.
func NodeToQX(node *domain.Node) string {
	if node == nil {
		return ""
	}
	payload := node.NormalizedPayload
	if payload == nil {
		payload = map[string]any{}
	}

	proto := strings.ToLower(string(node.CanonicalProtocol()))
	name := strings.TrimSpace(node.Name)
	server := strings.TrimSpace(node.Server)
	port := node.Port

	switch proto {
	case "ss", "shadowsocks":
		cipher := getString(payload, "cipher", "aes-256-gcm")
		password := getString(payload, "password", "")
		return fmt.Sprintf("shadowsocks=%s:%d, method=%s, password=%s, tag=%s", server, port, cipher, password, name)
	case "vmess":
		uuid := getString(payload, "uuid", "")
		tls := getBool(payload, "tls", false)
		net := getString(payload, "network", "tcp")
		wsPath := ""
		if wsOpts, ok := payload["ws-opts"].(map[string]any); ok {
			wsPath = getString(wsOpts, "path", "")
		} else if wsOpts, ok := payload["ws_opts"].(map[string]any); ok {
			wsPath = getString(wsOpts, "path", "")
		}

		parts := []string{fmt.Sprintf("vmess=%s:%d, method=none, password=%s", server, port, uuid)}
		if net == "ws" {
			parts = append(parts, "obfs=ws")
			if wsPath != "" {
				parts = append(parts, fmt.Sprintf("obfs-uri=%s", wsPath))
			}
		}
		if tls {
			parts = append(parts, "tls=true")
		}
		parts = append(parts, "fast-open=false", "udp-relay=false", fmt.Sprintf("tag=%s", name))
		return strings.Join(parts, ", ")
	case "trojan":
		password := getString(payload, "password", "")
		sni := getString(payload, "sni", server)
		parts := []string{fmt.Sprintf("trojan=%s:%d, password=%s, over-tls=true", server, port, password)}
		if sni != "" {
			parts = append(parts, fmt.Sprintf("tls-host=%s", sni))
		}
		parts = append(parts, fmt.Sprintf("tag=%s", name))
		return strings.Join(parts, ", ")
	case "hysteria2", "hy2":
		password := getString(payload, "password", "")
		sni := getString(payload, "sni", server)
		parts := []string{fmt.Sprintf("hysteria2=%s:%d, password=%s", server, port, password)}
		if sni != "" {
			parts = append(parts, fmt.Sprintf("sni=%s", sni))
		}
		parts = append(parts, fmt.Sprintf("tag=%s", name))
		return strings.Join(parts, ", ")
	case "vless":
		uuid := getString(payload, "uuid", "")
		return fmt.Sprintf("vless=%s:%d, method=none, password=%s, tag=%s", server, port, uuid, name)
	default:
		return ""
	}
}

func filterQXServer(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	if in == nil || in.IsNil() {
		return pongo2.AsSafeValue(""), nil
	}
	if n, ok := in.Interface().(*domain.Node); ok {
		return pongo2.AsSafeValue(NodeToQX(n)), nil
	}
	return pongo2.AsSafeValue(""), nil
}

// RuleToSurge formats a domain.Rule or Clash rule string into Surge rule syntax.
func RuleToSurge(val any) string {
	if val == nil {
		return ""
	}
	var r *domain.Rule
	switch v := val.(type) {
	case *domain.Rule:
		r = v
	case domain.Rule:
		r = &v
	case string:
		parsed, err := domain.ParseClashRule(v)
		if err == nil {
			r = parsed
		} else {
			return v
		}
	default:
		return ""
	}

	if r == nil || !r.Enabled {
		return ""
	}
	target := strings.TrimSpace(r.Proxy)
	if target == "" {
		target = "DIRECT"
	}
	rtype := strings.ToUpper(strings.TrimSpace(string(r.Type)))
	if rtype == "MATCH" {
		return fmt.Sprintf("FINAL,%s", target)
	}
	v := strings.TrimSpace(r.Value)
	if len(r.Options) > 0 {
		return fmt.Sprintf("%s,%s,%s,%s", rtype, v, target, strings.Join(r.Options, ","))
	}
	return fmt.Sprintf("%s,%s,%s", rtype, v, target)
}

func filterSurgeRule(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	if in == nil || in.IsNil() {
		return pongo2.AsSafeValue(""), nil
	}
	return pongo2.AsSafeValue(RuleToSurge(in.Interface())), nil
}

// RuleToLoon formats a domain.Rule or Clash rule string into Loon rule syntax.
func RuleToLoon(val any) string {
	if val == nil {
		return ""
	}
	var r *domain.Rule
	switch v := val.(type) {
	case *domain.Rule:
		r = v
	case domain.Rule:
		r = &v
	case string:
		parsed, err := domain.ParseClashRule(v)
		if err == nil {
			r = parsed
		} else {
			return v
		}
	default:
		return ""
	}

	if r == nil || !r.Enabled {
		return ""
	}
	target := strings.TrimSpace(r.Proxy)
	if target == "" {
		target = "DIRECT"
	}
	rtype := strings.ToUpper(strings.TrimSpace(string(r.Type)))
	if rtype == "MATCH" {
		return fmt.Sprintf("FINAL,%s", target)
	}
	v := strings.TrimSpace(r.Value)
	return fmt.Sprintf("%s,%s,%s", rtype, v, target)
}

func filterLoonRule(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	if in == nil || in.IsNil() {
		return pongo2.AsSafeValue(""), nil
	}
	return pongo2.AsSafeValue(RuleToLoon(in.Interface())), nil
}

// RuleToQX formats a domain.Rule or Clash rule string into Quantumult X filter_local syntax.
func RuleToQX(val any) string {
	if val == nil {
		return ""
	}
	var r *domain.Rule
	switch v := val.(type) {
	case *domain.Rule:
		r = v
	case domain.Rule:
		r = &v
	case string:
		parsed, err := domain.ParseClashRule(v)
		if err == nil {
			r = parsed
		} else {
			return v
		}
	default:
		return ""
	}

	if r == nil || !r.Enabled {
		return ""
	}
	target := strings.TrimSpace(r.Proxy)
	if target == "" {
		target = "DIRECT"
	}
	rtype := strings.ToUpper(strings.TrimSpace(string(r.Type)))
	v := strings.TrimSpace(r.Value)

	switch rtype {
	case "MATCH":
		return fmt.Sprintf("final, %s", target)
	case "DOMAIN":
		return fmt.Sprintf("host, %s, %s", v, target)
	case "DOMAIN-SUFFIX":
		return fmt.Sprintf("host-suffix, %s, %s", v, target)
	case "DOMAIN-KEYWORD":
		return fmt.Sprintf("host-keyword, %s, %s", v, target)
	case "IP-CIDR", "IP-CIDR6":
		return fmt.Sprintf("ip-cidr, %s, %s", v, target)
	case "GEOIP":
		return fmt.Sprintf("geoip, %s, %s", strings.ToLower(v), target)
	default:
		return fmt.Sprintf("host-suffix, %s, %s", v, target)
	}
}

func filterQXRule(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	if in == nil || in.IsNil() {
		return pongo2.AsSafeValue(""), nil
	}
	return pongo2.AsSafeValue(RuleToQX(in.Interface())), nil
}

// NodeGroupToSurge formats a node group into Surge proxy group line.
func NodeGroupToSurge(val any) string {
	if val == nil {
		return ""
	}
	name, groupType, proxies, url, interval, tolerance := extractGroupInfo(val)
	if name == "" {
		return ""
	}
	if len(proxies) == 0 {
		proxies = []string{"DIRECT"}
	}
	proxyList := strings.Join(proxies, ", ")

	switch groupType {
	case "url-test":
		return fmt.Sprintf("%s = url-test, %s, url=%s, interval=%d, tolerance=%d", name, proxyList, url, interval, tolerance)
	case "fallback":
		return fmt.Sprintf("%s = fallback, %s, url=%s, interval=%d", name, proxyList, url, interval)
	default:
		return fmt.Sprintf("%s = select, %s", name, proxyList)
	}
}

func filterSurgeGroup(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	if in == nil || in.IsNil() {
		return pongo2.AsSafeValue(""), nil
	}
	return pongo2.AsSafeValue(NodeGroupToSurge(in.Interface())), nil
}

// NodeGroupToLoon formats a node group into Loon proxy group line.
func NodeGroupToLoon(val any) string {
	if val == nil {
		return ""
	}
	name, groupType, proxies, url, interval, _ := extractGroupInfo(val)
	if name == "" {
		return ""
	}
	if len(proxies) == 0 {
		proxies = []string{"DIRECT"}
	}
	proxyList := strings.Join(proxies, ",")

	switch groupType {
	case "url-test":
		return fmt.Sprintf("%s = url-test,%s,url = %s,interval = %d", name, proxyList, url, interval)
	default:
		return fmt.Sprintf("%s = select,%s", name, proxyList)
	}
}

func filterLoonGroup(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	if in == nil || in.IsNil() {
		return pongo2.AsSafeValue(""), nil
	}
	return pongo2.AsSafeValue(NodeGroupToLoon(in.Interface())), nil
}

// NodeGroupToQX formats a node group into Quantumult X policy line.
func NodeGroupToQX(val any) string {
	if val == nil {
		return ""
	}
	name, groupType, proxies, url, _, tolerance := extractGroupInfo(val)
	if name == "" {
		return ""
	}
	if len(proxies) == 0 {
		proxies = []string{"DIRECT"}
	}
	proxyList := strings.Join(proxies, ", ")

	switch groupType {
	case "url-test":
		return fmt.Sprintf("url-latency-benchmark=%s, %s, check-url=%s, tolerance=%d", name, proxyList, url, tolerance)
	default:
		return fmt.Sprintf("static=%s, %s", name, proxyList)
	}
}

func filterQXPolicy(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	if in == nil || in.IsNil() {
		return pongo2.AsSafeValue(""), nil
	}
	return pongo2.AsSafeValue(NodeGroupToQX(in.Interface())), nil
}

func extractGroupInfo(val any) (name, groupType string, proxies []string, testURL string, interval, tolerance int) {
	testURL = "https://cp.cloudflare.com/generate_204"
	interval = 300
	tolerance = 50

	switch g := val.(type) {
	case *domain.NodeGroup:
		if g == nil {
			return
		}
		name = g.Name
		groupType = string(g.GroupType)
		proxies = g.ResolvedNodeNames
		cfg := g.EffectiveURLTestConfig()
		testURL = cfg.URL
		interval = cfg.Interval
		tolerance = cfg.Tolerance
	case domain.NodeGroup:
		name = g.Name
		groupType = string(g.GroupType)
		proxies = g.ResolvedNodeNames
		cfg := g.EffectiveURLTestConfig()
		testURL = cfg.URL
		interval = cfg.Interval
		tolerance = cfg.Tolerance
	case map[string]any:
		if n, ok := g["name"].(string); ok {
			name = n
		}
		if t, ok := g["type"].(string); ok {
			groupType = t
		}
		if p, ok := g["proxies"].([]string); ok {
			proxies = p
		} else if pSlice, ok := g["proxies"].([]any); ok {
			for _, item := range pSlice {
				if s, ok := item.(string); ok {
					proxies = append(proxies, s)
				}
			}
		}
		if u, ok := g["url"].(string); ok && u != "" {
			testURL = u
		}
		if iv, ok := g["interval"].(int); ok && iv > 0 {
			interval = iv
		}
		if tol, ok := g["tolerance"].(int); ok && tol > 0 {
			tolerance = tol
		}
	}
	return
}

// NodeToSingbox formats a node into a Sing-box outbound configuration map.
func NodeToSingbox(node *domain.Node) map[string]any {
	if node == nil {
		return nil
	}
	payload := node.NormalizedPayload
	if payload == nil {
		payload = map[string]any{}
	}

	proto := strings.ToLower(string(node.CanonicalProtocol()))
	name := strings.TrimSpace(node.Name)
	server := strings.TrimSpace(node.Server)
	port := node.Port

	switch proto {
	case "ss", "shadowsocks":
		return map[string]any{
			"type":        "shadowsocks",
			"tag":         name,
			"server":      server,
			"server_port": port,
			"method":      getString(payload, "cipher", "aes-256-gcm"),
			"password":    getString(payload, "password", ""),
		}
	case "vmess":
		out := map[string]any{
			"type":        "vmess",
			"tag":         name,
			"server":      server,
			"server_port": port,
			"uuid":        getString(payload, "uuid", ""),
			"security":    getString(payload, "cipher", "auto"),
			"alter_id":    getInt(payload, "alterId", 0),
		}
		if getBool(payload, "tls", false) {
			out["tls"] = map[string]any{"enabled": true}
		}
		if net := getString(payload, "network", ""); net == "ws" {
			wsPath := "/"
			if wsOpts, ok := payload["ws-opts"].(map[string]any); ok {
				wsPath = getString(wsOpts, "path", "/")
			}
			out["transport"] = map[string]any{
				"type": "ws",
				"path": wsPath,
			}
		}
		return out
	case "vless":
		out := map[string]any{
			"type":        "vless",
			"tag":         name,
			"server":      server,
			"server_port": port,
			"uuid":        getString(payload, "uuid", ""),
		}
		tlsMap := map[string]any{"enabled": true}
		if realityOpts, ok := payload["reality-opts"].(map[string]any); ok {
			tlsMap["reality"] = map[string]any{
				"enabled":    true,
				"public_key": getString(realityOpts, "public-key", ""),
				"short_id":   getString(realityOpts, "short-id", ""),
			}
		}
		if fp := getString(payload, "client-fingerprint", ""); fp != "" {
			tlsMap["utls"] = map[string]any{
				"enabled":     true,
				"fingerprint": fp,
			}
		}
		out["tls"] = tlsMap
		return out
	case "trojan":
		sni := getString(payload, "sni", server)
		return map[string]any{
			"type":        "trojan",
			"tag":         name,
			"server":      server,
			"server_port": port,
			"password":    getString(payload, "password", ""),
			"tls": map[string]any{
				"enabled":     true,
				"server_name": sni,
			},
		}
	case "hysteria2", "hy2":
		sni := getString(payload, "sni", server)
		return map[string]any{
			"type":        "hysteria2",
			"tag":         name,
			"server":      server,
			"server_port": port,
			"password":    getString(payload, "password", ""),
			"tls": map[string]any{
				"enabled":     true,
				"server_name": sni,
			},
		}
	case "tuic":
		sni := getString(payload, "sni", server)
		return map[string]any{
			"type":               "tuic",
			"tag":                name,
			"server":             server,
			"server_port":        port,
			"uuid":               getString(payload, "uuid", ""),
			"password":           getString(payload, "password", ""),
			"congestion_control": getString(payload, "congestion-controller", "bbr"),
			"tls": map[string]any{
				"enabled":     true,
				"server_name": sni,
			},
		}
	case "wireguard":
		ip := getString(payload, "ip", "10.0.0.2")
		if !strings.Contains(ip, "/") {
			ip = ip + "/32"
		}
		return map[string]any{
			"type":            "wireguard",
			"tag":             name,
			"server":          server,
			"server_port":     port,
			"local_address":   []string{ip},
			"private_key":     getString(payload, "private-key", ""),
			"peer_public_key": getString(payload, "public-key", ""),
		}
	default:
		return nil
	}
}

func filterSingboxOutbound(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	if in == nil || in.IsNil() {
		return pongo2.AsSafeValue(nil), nil
	}
	if n, ok := in.Interface().(*domain.Node); ok {
		return pongo2.AsSafeValue(NodeToSingbox(n)), nil
	}
	return pongo2.AsSafeValue(nil), nil
}

func getString(m map[string]any, key, dflt string) string {
	if v, ok := m[key]; ok && v != nil {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return dflt
}

func getInt(m map[string]any, key string, dflt int) int {
	if v, ok := m[key]; ok && v != nil {
		switch n := v.(type) {
		case int:
			return n
		case int64:
			return int(n)
		case float64:
			return int(n)
		}
	}
	return dflt
}

func getBool(m map[string]any, key string, dflt bool) bool {
	if v, ok := m[key]; ok && v != nil {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return dflt
}
