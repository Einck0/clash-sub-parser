package domain

import "strings"

func ValidateSubscriptionFetchSettings(mode, proxyURL string) error {
	switch mode {
	case "environment", "direct":
	case "proxy":
		if strings.TrimSpace(proxyURL) == "" {
			return NewValidationError("invalid_subscription_fetch", "proxy mode requires a proxy URL")
		}
	default:
		return NewValidationError("invalid_subscription_fetch", "mode must be environment, direct or proxy")
	}
	if len(proxyURL) > 4096 {
		return NewValidationError("invalid_proxy_url", "proxy URL exceeds 4096 bytes")
	}
	return nil
}

// EffectiveSubscriptionFetchProxy resolves the persisted mode against the startup CLI/environment fallback.
func (s Settings) EffectiveSubscriptionFetchProxy(environmentProxy string) string {
	switch s.SubscriptionFetchMode {
	case "direct":
		return ""
	case "proxy":
		return s.SubscriptionFetchProxyURL
	default:
		return environmentProxy
	}
}
