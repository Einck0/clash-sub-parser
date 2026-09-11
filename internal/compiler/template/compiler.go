package template

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
)

// Compiler implements compiler.Compiler using pongo2 template rendering.
type Compiler struct {
	engine *Engine
}

// NewCompiler constructs a Compiler backed by the embedded template engine.
func NewCompiler() (*Compiler, error) {
	engine, err := NewEngine()
	if err != nil {
		return nil, fmt.Errorf("failed to create template engine: %w", err)
	}
	return &Compiler{engine: engine}, nil
}

// NewCompilerWithEngine allows injecting a custom template engine.
func NewCompilerWithEngine(engine *Engine) *Compiler {
	return &Compiler{engine: engine}
}

// Compile compiles a bundle of nodes, groups, rules, and DNS into the target client configuration.
func (c *Compiler) Compile(ctx context.Context, bundle *compiler.Bundle, target domain.ExportTarget) (*compiler.Result, error) {
	if bundle == nil {
		return nil, fmt.Errorf("bundle cannot be nil")
	}

	normTarget := domain.ExportTarget(strings.ToLower(strings.TrimSpace(string(target))))
	if normTarget == domain.TargetShadowrocket {
		var lines []string
		for _, n := range bundle.Nodes {
			if link := NodeToShadowrocketLink(n); link != "" {
				lines = append(lines, link)
			}
		}
		joined := ""
		if len(lines) > 0 {
			joined = strings.Join(lines, "\n") + "\n"
		}
		b64Content := base64.StdEncoding.EncodeToString([]byte(joined))
		return &compiler.Result{
			Content:     []byte(b64Content),
			ContentType: "text/plain; charset=utf-8",
			Filename:    "sub.txt",
		}, nil
	}

	templateName, contentType, filename, err := resolveTargetParams(normTarget)
	if err != nil {
		return nil, err
	}

	templateContext := c.buildContext(bundle, normTarget)

	rendered, err := c.engine.Render(templateName, templateContext)
	if err != nil {
		return nil, fmt.Errorf("failed to render configuration for %s: %w", target, err)
	}

	// Clean trailing white spaces
	cleanContent := strings.TrimRight(rendered, "\r\n ") + "\n"

	return &compiler.Result{
		Content:     []byte(cleanContent),
		ContentType: contentType,
		Filename:    filename,
	}, nil
}

func resolveTargetParams(target domain.ExportTarget) (templateName, contentType, filename string, err error) {
	switch target {
	case domain.TargetClash:
		return "clash.yaml.p2", "application/x-yaml; charset=utf-8", "clash.yaml", nil
	case domain.TargetMihomo:
		return "mihomo.yaml.p2", "application/x-yaml; charset=utf-8", "mihomo.yaml", nil
	case domain.TargetStash:
		return "clash.yaml.p2", "application/x-yaml; charset=utf-8", "stash.yaml", nil
	case domain.TargetSingBox:
		return "singbox.json.p2", "application/json; charset=utf-8", "config.json", nil
	case domain.TargetSurge:
		return "surge.conf.p2", "text/plain; charset=utf-8", "surge.conf", nil
	case domain.TargetLoon:
		return "loon.conf.p2", "text/plain; charset=utf-8", "loon.conf", nil
	case domain.TargetQuantumultX:
		return "qx.conf.p2", "text/plain; charset=utf-8", "quantumult-x.conf", nil
	case domain.TargetShadowrocket:
		return "", "text/plain; charset=utf-8", "sub.txt", nil
	default:
		return "", "", "", fmt.Errorf("%w: %s", domain.ErrUnsupportedExportTarget, target)
	}
}

func (c *Compiler) buildContext(bundle *compiler.Bundle, target domain.ExportTarget) map[string]any {
	ctx := make(map[string]any)

	// 1. Sanitized nodes for Clash / Mihomo / Stash
	sanitizedProxies := make([]map[string]any, 0, len(bundle.Nodes))
	for _, n := range bundle.Nodes {
		if n != nil {
			sanitizedProxies = append(sanitizedProxies, n.SanitizeForExport())
		}
	}
	ctx["proxies"] = sanitizedProxies
	ctx["nodes"] = bundle.Nodes

	// 2. Proxy groups
	proxyGroups := make([]map[string]any, 0, len(bundle.Groups))
	effectiveGroups := make([]*domain.NodeGroup, 0, len(bundle.Groups))
	for _, g := range bundle.Groups {
		if g == nil {
			continue
		}
		gCopy := *g
		proxies := g.ResolvedNodeNames
		if len(proxies) == 0 {
			proxies = []string{"DIRECT"}
		}
		gCopy.ResolvedNodeNames = proxies
		effectiveGroups = append(effectiveGroups, &gCopy)

		grpMap := map[string]any{
			"name":    g.Name,
			"type":    string(g.GroupType),
			"proxies": proxies,
		}
		if g.GroupType == domain.GroupTypeURLTest || g.GroupType == domain.GroupTypeFallback || g.GroupType == domain.GroupTypeLoadBalance {
			urlCfg := g.EffectiveURLTestConfig()
			grpMap["url"] = urlCfg.URL
			grpMap["interval"] = urlCfg.Interval
			if urlCfg.Tolerance > 0 {
				grpMap["tolerance"] = urlCfg.Tolerance
			}
		}
		proxyGroups = append(proxyGroups, grpMap)
	}
	ctx["proxy_groups"] = proxyGroups
	ctx["groups"] = effectiveGroups

	// 3. Rules
	clashRules := make([]string, 0, len(bundle.Rules))
	effectiveRules := make([]*domain.Rule, 0, len(bundle.Rules))
	finalTarget := "DIRECT"
	for _, r := range bundle.Rules {
		if r == nil || !r.Enabled {
			continue
		}
		effectiveRules = append(effectiveRules, r)
		clashRules = append(clashRules, r.ToClashFormat())
		if strings.ToUpper(strings.TrimSpace(string(r.Type))) == "MATCH" {
			if strings.TrimSpace(r.Proxy) != "" {
				finalTarget = strings.TrimSpace(r.Proxy)
			}
		}
	}
	ctx["rules"] = clashRules
	ctx["rules_objects"] = effectiveRules

	// 4. DNS
	if bundle.DNS != nil && bundle.DNS.Enabled {
		ctx["dns"] = bundle.DNS
		ctx["dns_servers"] = bundle.DNS.Nameservers
	} else {
		ctx["dns_servers"] = []string{"223.5.5.5", "119.29.29.29", "1.1.1.1"}
	}

	// 5. Sing-box specific configuration
	if target == domain.TargetSingBox {
		c.buildSingboxContext(ctx, bundle, effectiveGroups, effectiveRules, finalTarget)
	}

	return ctx
}

func (c *Compiler) buildSingboxContext(
	ctx map[string]any,
	bundle *compiler.Bundle,
	groups []*domain.NodeGroup,
	rules []*domain.Rule,
	finalTarget string,
) {
	// Inbounds
	inbounds := []map[string]any{
		{
			"type":        "mixed",
			"tag":         "mixed-in",
			"listen":      "127.0.0.1",
			"listen_port": 2080,
		},
	}
	ctx["inbounds"] = inbounds

	// Outbounds
	outbounds := []map[string]any{
		{"type": "direct", "tag": "DIRECT"},
		{"type": "block", "tag": "REJECT"},
	}

	for _, n := range bundle.Nodes {
		if sbOut := NodeToSingbox(n); sbOut != nil {
			outbounds = append(outbounds, sbOut)
		}
	}

	for _, g := range groups {
		proxies := g.ResolvedNodeNames
		if len(proxies) == 0 {
			proxies = []string{"DIRECT"}
		}
		if g.GroupType == domain.GroupTypeURLTest || g.GroupType == domain.GroupTypeFallback {
			cfg := g.EffectiveURLTestConfig()
			outbounds = append(outbounds, map[string]any{
				"type":      "urltest",
				"tag":       g.Name,
				"outbounds": proxies,
				"url":       cfg.URL,
				"interval":  fmt.Sprintf("%ds", cfg.Interval),
				"tolerance": cfg.Tolerance,
			})
		} else {
			outbounds = append(outbounds, map[string]any{
				"type":      "selector",
				"tag":       g.Name,
				"outbounds": proxies,
			})
		}
	}
	ctx["outbounds"] = outbounds

	// Route rules
	routeRules := make([]map[string]any, 0, len(rules))
	for _, r := range rules {
		rtype := strings.ToUpper(strings.TrimSpace(string(r.Type)))
		target := strings.TrimSpace(r.Proxy)
		if target == "" {
			target = "DIRECT"
		}
		if rtype == "MATCH" {
			continue
		}

		val := strings.TrimSpace(r.Value)
		ruleObj := map[string]any{"outbound": target}
		switch rtype {
		case "DOMAIN":
			ruleObj["domain"] = []string{val}
		case "DOMAIN-SUFFIX":
			ruleObj["domain_suffix"] = []string{val}
		case "DOMAIN-KEYWORD":
			ruleObj["domain_keyword"] = []string{val}
		case "IP-CIDR", "IP-CIDR6":
			ruleObj["ip_cidr"] = []string{val}
		case "GEOSITE":
			ruleObj["geosite"] = []string{val}
		case "GEOIP":
			ruleObj["geoip"] = []string{val}
		default:
			ruleObj["domain_suffix"] = []string{val}
		}
		routeRules = append(routeRules, ruleObj)
	}

	ctx["route"] = map[string]any{
		"rules": routeRules,
		"final": finalTarget,
	}

	// DNS
	dnsServers := []map[string]any{
		{
			"tag":     "dns-remote",
			"address": "https://1.1.1.1/dns-query",
			"detour":  "DIRECT",
		},
		{
			"tag":     "dns-direct",
			"address": "223.5.5.5",
			"detour":  "DIRECT",
		},
	}
	ctx["dns_config"] = map[string]any{
		"servers": dnsServers,
		"rules": []map[string]any{
			{
				"outbound": "any",
				"server":   "dns-direct",
			},
		},
	}
}
