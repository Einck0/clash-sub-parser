package domain

import "strings"

// NormalizeTransportNetwork interprets Xray's VLESS raw name as TCP at consumers.
// It does not modify stored credentials or change other protocols' semantics.
func NormalizeTransportNetwork(protocol Protocol, network string) string {
	network = strings.ToLower(strings.TrimSpace(network))
	if protocol == ProtocolVLESS && network == "raw" {
		return "tcp"
	}
	return network
}
