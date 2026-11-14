package mihomo_test

import (
	"reflect"
	"testing"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/mihomo"
)

func TestNodeToMapping_VLESSStoredRawUsesTCPWithoutChangingOtherFields(t *testing.T) {
	raw := domain.Node{
		LogicalID:   "synthetic-vless-raw-revision-1",
		DisplayName: "Synthetic VLESS Raw",
		Protocol:    domain.ProtocolVLESS,
		Server:      "vless.synthetic.invalid",
		Port:        443,
		Credentials: domain.InboundProtocolCredential{
			UUID: "11111111-2222-4333-8444-555555555555",
			Transport: map[string]string{
				"network": "raw",
				"tls":     "true",
				"sni":     "sni.synthetic.invalid",
				"flow":    "xtls-rprx-vision",
				"pbk":     "synthetic-public-key",
				"sid":     "abcd",
			},
		},
	}
	tcp := raw
	tcp.Credentials.Transport = make(map[string]string, len(raw.Credentials.Transport))
	for key, value := range raw.Credentials.Transport {
		tcp.Credentials.Transport[key] = value
	}
	tcp.Credentials.Transport["network"] = "tcp"

	rawMapping, err := mihomo.NodeToMapping(raw)
	if err != nil {
		t.Fatalf("map stored VLESS raw: %v", err)
	}
	tcpMapping, err := mihomo.NodeToMapping(tcp)
	if err != nil {
		t.Fatalf("map VLESS tcp: %v", err)
	}
	if rawMapping["network"] != "tcp" {
		t.Errorf("probe adapter network = %v, want tcp", rawMapping["network"])
	}
	if !reflect.DeepEqual(rawMapping, tcpMapping) {
		t.Errorf("raw and tcp probe mappings differ:\nraw: %#v\ntcp: %#v", rawMapping, tcpMapping)
	}
	if raw.Credentials.Transport["network"] != "raw" {
		t.Errorf("probe adapter mutated stored transport network to %q", raw.Credentials.Transport["network"])
	}
	if raw.LogicalID != "synthetic-vless-raw-revision-1" {
		t.Errorf("probe adapter changed logical identity: %q", raw.LogicalID)
	}
}

func TestNodeToMapping_UnknownVLESSTransportIsPassedThroughForConsumerValidation(t *testing.T) {
	node := domain.Node{
		LogicalID: "synthetic-vless-unknown-transport",
		Protocol:  domain.ProtocolVLESS,
		Server:    "vless.synthetic.invalid",
		Port:      443,
		Credentials: domain.InboundProtocolCredential{
			UUID:      "11111111-2222-4333-8444-555555555555",
			Transport: map[string]string{"network": "unrecognized-transport"},
		},
	}
	mapping, err := mihomo.NodeToMapping(node)
	if err != nil {
		t.Fatalf("mapping should not claim to validate target capabilities: %v", err)
	}
	if mapping["network"] != "unrecognized-transport" {
		t.Fatalf("unknown network was unexpectedly rewritten: %v", mapping["network"])
	}
}

func TestNodeToMapping_NonVLESSRawNetworkIsUnchanged(t *testing.T) {
	node := domain.Node{
		Protocol: domain.ProtocolTrojan,
		Server:   "trojan.synthetic.invalid",
		Port:     443,
		Credentials: domain.InboundProtocolCredential{
			Password:  "synthetic-password",
			Transport: map[string]string{"network": "raw"},
		},
	}
	mapping, err := mihomo.NodeToMapping(node)
	if err != nil {
		t.Fatalf("map Trojan raw network: %v", err)
	}
	if mapping["network"] != "raw" {
		t.Fatalf("non-VLESS network = %v, want unchanged raw", mapping["network"])
	}
}
