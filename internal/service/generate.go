package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository"
)

// generateServiceImpl implements GenerateService.
type generateServiceImpl struct {
	repos    *repository.Repositories
	compiler compiler.Compiler
}

// NewGenerateService creates a new instance of GenerateService.
func NewGenerateService(repos *repository.Repositories, comp compiler.Compiler) GenerateService {
	return &generateServiceImpl{
		repos:    repos,
		compiler: comp,
	}
}

// Generate implements GenerateService.Generate.
func (s *generateServiceImpl) Generate(ctx context.Context, req *domain.GenerateRequest) ([]byte, string, error) {
	res, err := s.GenerateResult(ctx, req, nil, "")
	if err != nil {
		return nil, "", err
	}
	return res.Content, res.ContentType, nil
}

// GetConfig returns the current generation switches/settings.
func (s *generateServiceImpl) GetConfig(ctx context.Context) (*domain.GenerateConfig, error) {
	cfg, err := s.repos.GenerateConfig.Get(ctx)
	if err != nil {
		return domain.DefaultGenerateConfig(), nil
	}
	return cfg, nil
}

// UpdateConfig updates the generation configuration.
func (s *generateServiceImpl) UpdateConfig(ctx context.Context, cfg *domain.GenerateConfig) error {
	return s.repos.GenerateConfig.Update(ctx, cfg)
}

// GenerateResult builds the bundle and compiles the target client configuration.
func (s *generateServiceImpl) GenerateResult(
	ctx context.Context,
	req *domain.GenerateRequest,
	subID *int64,
	filterKeyword string,
) (*compiler.Result, error) {
	if req == nil {
		req = domain.DefaultGenerateRequest(domain.TargetClash)
	}

	normTarget := domain.ExportTarget(strings.ToLower(strings.TrimSpace(string(req.Target))))
	if err := (&domain.GenerateRequest{Target: normTarget}).Validate(); err != nil {
		return nil, fmt.Errorf("%w: %s", domain.ErrUnsupportedExportTarget, req.Target)
	}

	// 1. Generate config switches
	genCfg, _ := s.repos.GenerateConfig.Get(ctx)
	if genCfg == nil {
		genCfg = domain.DefaultGenerateConfig()
	}

	// 2. Collect nodes
	nodes, err := s.collectNodes(ctx, subID, req.IncludeUnchecked, filterKeyword)
	if err != nil {
		return nil, err
	}

	// 3. Collect groups
	var groups []*domain.NodeGroup
	if genCfg.NodeGroups && (req.Switches == nil || req.Switches["node_groups"]) {
		groups, err = s.collectGroups(ctx, nodes, req.FilterGroups)
		if err != nil {
			return nil, err
		}
	}

	// 4. Collect rules
	var rules []*domain.Rule
	if genCfg.Rules && (req.Switches == nil || req.Switches["rules"]) {
		rules, err = s.repos.Rules.List(ctx, true)
		if err != nil {
			return nil, err
		}
	}

	// 5. Collect DNS
	var dns *domain.DNSConfig
	if genCfg.DNS && (req.Switches == nil || req.Switches["dns"]) {
		dns, _ = s.repos.DNSConfig.Get(ctx)
	}

	bundle := &compiler.Bundle{
		Nodes:          nodes,
		Groups:         groups,
		Rules:          rules,
		DNS:            dns,
		GenerateConfig: genCfg,
	}

	return s.compiler.Compile(ctx, bundle, normTarget)
}

func (s *generateServiceImpl) collectNodes(
	ctx context.Context,
	subID *int64,
	includeUnchecked bool,
	filterKeyword string,
) ([]*domain.Node, error) {
	var rawNodes []*domain.Node

	if subID != nil {
		sub, err := s.repos.Subscriptions.GetByID(ctx, *subID)
		if err != nil {
			return nil, err
		}
		if len(sub.RawNodes) > 0 {
			rawNodes = append(rawNodes, sub.RawNodes...)
		} else if len(sub.SourceNodes) > 0 {
			rawNodes = append(rawNodes, sub.SourceNodes...)
		} else if len(sub.ManualNodes) > 0 {
			rawNodes = append(rawNodes, sub.ManualNodes...)
		}
	} else {
		nodeCount, _ := s.repos.Nodes.Count(ctx)
		if nodeCount > 0 {
			state := domain.LifecycleActive
			if includeUnchecked {
				state = ""
			}
			var err error
			rawNodes, err = s.repos.Nodes.List(ctx, state)
			if err != nil {
				return nil, err
			}
		} else {
			subs, err := s.repos.Subscriptions.List(ctx, true)
			if err != nil {
				return nil, err
			}
			for _, sub := range subs {
				if len(sub.RawNodes) > 0 {
					rawNodes = append(rawNodes, sub.RawNodes...)
				} else if len(sub.SourceNodes) > 0 {
					rawNodes = append(rawNodes, sub.SourceNodes...)
				}
			}
		}
	}

	if strings.TrimSpace(filterKeyword) == "" {
		return rawNodes, nil
	}

	trimmedFilter := strings.TrimSpace(filterKeyword)
	re, regErr := regexp.Compile("(?i)" + trimmedFilter)

	var filtered []*domain.Node
	for _, n := range rawNodes {
		if n == nil {
			continue
		}
		matched := false
		if regErr == nil {
			matched = re.MatchString(n.Name) || re.MatchString(n.Server)
		} else {
			matched = strings.Contains(strings.ToLower(n.Name), strings.ToLower(trimmedFilter)) ||
				strings.Contains(strings.ToLower(n.Server), strings.ToLower(trimmedFilter))
		}
		if matched {
			filtered = append(filtered, n)
		}
	}

	return filtered, nil
}

func (s *generateServiceImpl) collectGroups(
	ctx context.Context,
	nodes []*domain.Node,
	filterGroups []string,
) ([]*domain.NodeGroup, error) {
	allGroups, err := s.repos.NodeGroups.List(ctx)
	if err != nil {
		return nil, err
	}

	filterMap := make(map[string]bool)
	for _, fg := range filterGroups {
		if strings.TrimSpace(fg) != "" {
			filterMap[strings.ToLower(strings.TrimSpace(fg))] = true
		}
	}

	var result []*domain.NodeGroup
	for _, g := range allGroups {
		if g == nil {
			continue
		}
		if len(filterMap) > 0 && !filterMap[strings.ToLower(g.Name)] {
			continue
		}

		gCopy := *g
		if len(gCopy.ResolvedNodeNames) == 0 {
			var resolved []string
			// Match nodes against group regex
			for _, n := range nodes {
				if n == nil {
					continue
				}
				if matchesGroup(g, n.Name) {
					resolved = append(resolved, n.Name)
				}
			}
			if len(resolved) == 0 {
				resolved = []string{"DIRECT"}
			}
			gCopy.ResolvedNodeNames = resolved
		}
		result = append(result, &gCopy)
	}

	return result, nil
}

func matchesGroup(g *domain.NodeGroup, nodeName string) bool {
	if len(g.RegexRules) == 0 {
		return true
	}
	for _, pattern := range g.RegexRules {
		if strings.TrimSpace(pattern) == "" {
			continue
		}
		if re, err := regexp.Compile(pattern); err == nil && re.MatchString(nodeName) {
			return true
		}
	}
	return false
}

// GetPrimarySubscriptionHeaders extracts subscription traffic, update interval, and web page headers.
func (s *generateServiceImpl) GetPrimarySubscriptionHeaders(ctx context.Context) (map[string]string, error) {
	subs, err := s.repos.Subscriptions.List(ctx, true)
	if err != nil {
		return nil, err
	}

	var primary *domain.Subscription
	for _, sub := range subs {
		if sub.IsPrimary {
			primary = sub
			break
		}
	}
	if primary == nil && len(subs) > 0 {
		primary = subs[0]
	}

	headers := make(map[string]string)
	if primary != nil {
		if strings.TrimSpace(primary.SubscriptionUserinfo) != "" {
			headers["Subscription-Userinfo"] = strings.TrimSpace(primary.SubscriptionUserinfo)
		}
		if strings.TrimSpace(primary.ProfileUpdateInterval) != "" {
			headers["Profile-Update-Interval"] = strings.TrimSpace(primary.ProfileUpdateInterval)
		} else {
			headers["Profile-Update-Interval"] = "24"
		}
		if strings.TrimSpace(primary.ProfileWebPageURL) != "" {
			headers["Profile-Web-Page-Url"] = strings.TrimSpace(primary.ProfileWebPageURL)
		}
	} else {
		headers["Profile-Update-Interval"] = "24"
	}

	return headers, nil
}

// BuildQuickExport produces multi-client subscription URLs, import schemes, and QR payloads.
func (s *generateServiceImpl) BuildQuickExport(
	ctx context.Context,
	baseURL string,
	token string,
	subID *int64,
	targetFilter string,
) (map[string]any, error) {
	cleanBaseURL := strings.TrimRight(baseURL, "/")

	targetDefs := []struct {
		Target string
		Name   string
		Ext    string
	}{
		{"clash", "Clash", "yaml"},
		{"mihomo", "Mihomo", "yaml"},
		{"stash", "Stash", "yaml"},
		{"sing-box", "Sing-box", "json"},
		{"surge", "Surge", "conf"},
		{"loon", "Loon", "conf"},
		{"quantumult-x", "Quantumult X", "conf"},
		{"shadowrocket", "Shadowrocket", "txt"},
	}

	scope := "merged"
	subName := "ClashSubParser"
	if subID != nil {
		sub, err := s.repos.Subscriptions.GetByID(ctx, *subID)
		if err != nil {
			return nil, err
		}
		scope = "subscription"
		if strings.TrimSpace(sub.Name) != "" {
			subName = sub.Name
		} else {
			subName = fmt.Sprintf("Sub-%d", *subID)
		}
	}

	var items []map[string]any
	targetsMap := make(map[string]any)

	for _, td := range targetDefs {
		tKey := td.Target
		tName := td.Name

		var exportURL string
		if scope == "subscription" {
			qParts := []string{fmt.Sprintf("target=%s", tKey)}
			if token != "" {
				qParts = append(qParts, fmt.Sprintf("token=%s", token))
			}
			exportURL = fmt.Sprintf("%s/api/generate/subscription/%d?%s", cleanBaseURL, *subID, strings.Join(qParts, "&"))
		} else {
			qStr := ""
			if token != "" {
				qStr = fmt.Sprintf("?token=%s", token)
			}
			exportURL = fmt.Sprintf("%s/api/generate/%s%s", cleanBaseURL, tKey, qStr)
		}

		var schemeURL string
		var qrPayload string

		switch tKey {
		case "clash", "mihomo":
			schemeURL = fmt.Sprintf("clash://install-config?url=%s&name=%s", url.QueryEscape(exportURL), url.QueryEscape(subName))
			qrPayload = exportURL
		case "stash":
			schemeURL = fmt.Sprintf("stash://install-config?url=%s&name=%s", url.QueryEscape(exportURL), url.QueryEscape(subName))
			qrPayload = exportURL
		case "sing-box":
			schemeURL = fmt.Sprintf("sing-box://import-remote-profile?url=%s#%s", url.QueryEscape(exportURL), url.QueryEscape(subName))
			qrPayload = exportURL
		case "surge":
			schemeURL = fmt.Sprintf("surge:///install-config?url=%s", url.QueryEscape(exportURL))
			qrPayload = exportURL
		case "loon":
			schemeURL = fmt.Sprintf("loon://import?nodelist=%s", url.QueryEscape(exportURL))
			qrPayload = exportURL
		case "quantumult-x":
			schemeURL = fmt.Sprintf("quantumult-x:///add-resource?remote-resource=%s", url.QueryEscape(exportURL))
			qrPayload = exportURL
		case "shadowrocket":
			b64Sub := base64.StdEncoding.EncodeToString([]byte(exportURL))
			schemeURL = fmt.Sprintf("sub://%s", b64Sub)
			qrPayload = schemeURL
		default:
			schemeURL = exportURL
			qrPayload = exportURL
		}

		item := map[string]any{
			"target":          tKey,
			"name":            tName,
			"url":             exportURL,
			"scheme_url":      schemeURL,
			"qrcode_payload":  qrPayload,
		}
		items = append(items, item)
		targetsMap[tKey] = item
	}

	resp := map[string]any{
		"scope":             scope,
		"subscription_id":   subID,
		"subscription_name": nil,
		"targets":           targetsMap,
		"items":             items,
	}
	if scope == "subscription" {
		resp["subscription_name"] = subName
	}

	if targetFilter != "" {
		tf := strings.ToLower(strings.TrimSpace(targetFilter))
		if sel, ok := targetsMap[tf]; ok {
			resp["selected_target"] = sel
		}
	}

	return resp, nil
}
