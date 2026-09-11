package template

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"clash-sub-parser/internal/domain"
)

// NodeToShadowrocketLink converts a domain.Node into a protocol URI string.
// Returns an empty string if node cannot be serialized or lacks required fields.
func NodeToShadowrocketLink(node *domain.Node) string {
	if node == nil {
		return ""
	}

	cleaned := node.SanitizeForExport()
	proto := strings.ToLower(strings.TrimSpace(string(node.Protocol)))
	server := strings.TrimSpace(node.Server)
	port := node.Port
	name := strings.TrimSpace(node.Name)
	if name == "" {
		name = fmt.Sprintf("%s:%d", server, port)
	}
	nameEncoded := url.QueryEscape(name)

	if server == "" || port <= 0 {
		return ""
	}

	switch proto {
	case "ss", "shadowsocks":
		cipher := getString(cleaned, "cipher", "aes-256-gcm")
		pwd := getString(cleaned, "password", "")
		userinfo := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", cipher, pwd)))
		link := fmt.Sprintf("ss://%s@%s:%d#%s", userinfo, server, port, nameEncoded)
		plugin := getString(cleaned, "plugin", "")
		if plugin != "" {
			optStr := ""
			if pluginOpts, ok := cleaned["plugin-opts"].(map[string]any); ok {
				var pairs []string
				for k, v := range pluginOpts {
					pairs = append(pairs, fmt.Sprintf("%s=%v", k, v))
				}
				optStr = strings.Join(pairs, ";")
			}
			link += fmt.Sprintf("?plugin=%s", url.QueryEscape(fmt.Sprintf("%s;%s", plugin, optStr)))
		}
		return link

	case "vmess":
		uuidStr := getString(cleaned, "uuid", "")
		if uuidStr == "" {
			return ""
		}
		aid := getInt(cleaned, "alterId", 0)
		scy := getString(cleaned, "cipher", "auto")
		net := getString(cleaned, "network", "tcp")
		host := getString(cleaned, "servername", getString(cleaned, "sni", ""))
		tlsStr := ""
		if getBool(cleaned, "tls", false) {
			tlsStr = "tls"
		}
		vmessPayload := map[string]any{
			"v":    "2",
			"ps":   name,
			"add":  server,
			"port": port,
			"id":   uuidStr,
			"aid":  aid,
			"scy":  scy,
			"net":  net,
			"type": "none",
			"host": host,
			"tls":  tlsStr,
		}
		if wsOpts, ok := cleaned["ws-opts"].(map[string]any); ok {
			if path, ok := wsOpts["path"].(string); ok && path != "" {
				vmessPayload["path"] = path
			}
		}
		jsonBytes, err := json.Marshal(vmessPayload)
		if err != nil {
			return ""
		}
		return fmt.Sprintf("vmess://%s", base64.StdEncoding.EncodeToString(jsonBytes))

	case "vless":
		uuidStr := getString(cleaned, "uuid", "")
		if uuidStr == "" {
			return ""
		}
		sni := getString(cleaned, "sni", getString(cleaned, "servername", server))
		var params []string
		if getBool(cleaned, "tls", false) {
			params = append(params, "security=tls")
		}
		if realityOpts, ok := cleaned["reality-opts"].(map[string]any); ok {
			if pbk, ok := realityOpts["public-key"].(string); ok && pbk != "" {
				params = []string{"security=reality", fmt.Sprintf("pbk=%s", pbk)}
				if sid, ok := realityOpts["short-id"].(string); ok && sid != "" {
					params = append(params, fmt.Sprintf("sid=%s", sid))
				}
			}
		}
		if sni != "" {
			params = append(params, fmt.Sprintf("sni=%s", url.QueryEscape(sni)))
		}
		flow := getString(cleaned, "flow", "")
		if flow != "" {
			params = append(params, fmt.Sprintf("flow=%s", url.QueryEscape(flow)))
		}
		net := getString(cleaned, "network", "")
		if net != "" {
			params = append(params, fmt.Sprintf("type=%s", url.QueryEscape(net)))
		}
		paramStr := ""
		if len(params) > 0 {
			paramStr = "?" + strings.Join(params, "&")
		}
		return fmt.Sprintf("vless://%s@%s:%d%s#%s", uuidStr, server, port, paramStr, nameEncoded)

	case "trojan":
		pwd := getString(cleaned, "password", "")
		if pwd == "" {
			return ""
		}
		sni := getString(cleaned, "sni", getString(cleaned, "servername", server))
		var params []string
		if sni != "" {
			params = append(params, fmt.Sprintf("sni=%s", url.QueryEscape(sni)))
		}
		if getBool(cleaned, "skip-cert-verify", false) {
			params = append(params, "allowInsecure=1")
		}
		paramStr := ""
		if len(params) > 0 {
			paramStr = "?" + strings.Join(params, "&")
		}
		return fmt.Sprintf("trojan://%s@%s:%d%s#%s", pwd, server, port, paramStr, nameEncoded)

	case "hysteria2", "hy2":
		pwd := getString(cleaned, "password", getString(cleaned, "auth", ""))
		if pwd == "" {
			return ""
		}
		sni := getString(cleaned, "sni", getString(cleaned, "servername", server))
		var params []string
		if sni != "" {
			params = append(params, fmt.Sprintf("sni=%s", url.QueryEscape(sni)))
		}
		if getBool(cleaned, "skip-cert-verify", false) {
			params = append(params, "insecure=1")
		}
		obfs := getString(cleaned, "obfs", "")
		if obfs != "" {
			params = append(params, fmt.Sprintf("obfs=%s", url.QueryEscape(obfs)))
			obfsPwd := getString(cleaned, "obfs-password", "")
			if obfsPwd != "" {
				params = append(params, fmt.Sprintf("obfs-password=%s", url.QueryEscape(obfsPwd)))
			}
		}
		paramStr := ""
		if len(params) > 0 {
			paramStr = "?" + strings.Join(params, "&")
		}
		return fmt.Sprintf("hysteria2://%s@%s:%d%s#%s", pwd, server, port, paramStr, nameEncoded)

	case "tuic":
		uuidStr := getString(cleaned, "uuid", "")
		pwd := getString(cleaned, "password", "")
		if uuidStr == "" && pwd == "" {
			return ""
		}
		sni := getString(cleaned, "sni", getString(cleaned, "servername", server))
		var params []string
		if sni != "" {
			params = append(params, fmt.Sprintf("sni=%s", url.QueryEscape(sni)))
		}
		cc := getString(cleaned, "congestion-controller", "")
		if cc != "" {
			params = append(params, fmt.Sprintf("congestion_control=%s", url.QueryEscape(cc)))
		}
		paramStr := ""
		if len(params) > 0 {
			paramStr = "?" + strings.Join(params, "&")
		}
		auth := uuidStr
		if pwd != "" {
			if auth != "" {
				auth = fmt.Sprintf("%s:%s", auth, pwd)
			} else {
				auth = pwd
			}
		}
		return fmt.Sprintf("tuic://%s@%s:%d%s#%s", auth, server, port, paramStr, nameEncoded)

	case "wireguard", "wg":
		privKey := getString(cleaned, "private-key", getString(cleaned, "private_key", ""))
		pubKey := getString(cleaned, "public-key", getString(cleaned, "public_key", ""))
		ip := getString(cleaned, "ip", "10.0.0.2")
		var params []string
		if pubKey != "" {
			params = append(params, fmt.Sprintf("publickey=%s", url.QueryEscape(pubKey)))
		}
		if ip != "" {
			params = append(params, fmt.Sprintf("address=%s", url.QueryEscape(ip)))
		}
		mtu := getInt(cleaned, "mtu", 0)
		if mtu > 0 {
			params = append(params, fmt.Sprintf("mtu=%d", mtu))
		}
		reserved := getString(cleaned, "reserved", "")
		if reserved != "" {
			params = append(params, fmt.Sprintf("reserved=%s", url.QueryEscape(reserved)))
		}
		paramStr := ""
		if len(params) > 0 {
			paramStr = "?" + strings.Join(params, "&")
		}
		return fmt.Sprintf("wireguard://%s@%s:%d%s#%s", privKey, server, port, paramStr, nameEncoded)

	default:
		return ""
	}
}
