package domain_test

import (
	"encoding/json"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"gopkg.in/yaml.v3"
)

func TestNode_Protocols(t *testing.T) {
	// 1. Shadowsocks
	ssNode := &domain.Node{
		LogicalID: "node-ss-1",
		Name:      "SS-HK-01",
		Protocol:  domain.ProtocolShadowsocks,
		Server:    "1.2.3.4",
		Port:      8388,
	}
	ssOpts := domain.ShadowsocksOptions{
		Cipher:     "aes-256-gcm",
		Password:   "secret123",
		Plugin:     "v2ray-plugin",
		PluginOpts: map[string]any{"mode": "websocket", "host": "hk.example.com"},
		UDP:        true,
	}
	if err := ssNode.SetProtocolOptions(ssOpts); err != nil {
		t.Fatalf("SetProtocolOptions(ss) failed: %v", err)
	}
	if err := ssNode.Validate(); err != nil {
		t.Fatalf("ssNode.Validate() failed: %v", err)
	}
	gotSS, err := ssNode.GetShadowsocksOptions()
	if err != nil || gotSS.Cipher != "aes-256-gcm" || gotSS.Password != "secret123" {
		t.Fatalf("GetShadowsocksOptions() mismatch: %+v, err: %v", gotSS, err)
	}

	// 2. VMess
	vmessNode := &domain.Node{
		LogicalID: "node-vmess-1",
		Name:      "VMess-US-01",
		Protocol:  domain.ProtocolVMess,
		Server:    "us.example.com",
		Port:      443,
	}
	vmessOpts := domain.VMessOptions{
		UUID:           "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		AlterID:        0,
		Cipher:         "auto",
		Network:        "ws",
		TLS:            true,
		SNI:            "us.example.com",
		SkipCertVerify: false,
		WSOptions: &domain.WSOptions{
			Path:    "/ws",
			Headers: map[string]string{"Host": "us.example.com"},
		},
	}
	if err := vmessNode.SetProtocolOptions(vmessOpts); err != nil {
		t.Fatalf("SetProtocolOptions(vmess) failed: %v", err)
	}
	if err := vmessNode.Validate(); err != nil {
		t.Fatalf("vmessNode.Validate() failed: %v", err)
	}

	// 3. VLESS (with Reality and uTLS)
	vlessNode := &domain.Node{
		LogicalID: "node-vless-1",
		Name:      "VLESS-Reality-JP-01",
		Protocol:  domain.ProtocolVLESS,
		Server:    "jp.example.com",
		Port:      443,
	}
	vlessOpts := domain.VLESSOptions{
		UUID:    "11223344-5566-7788-99aa-bbccddeeff00",
		Flow:    "xtls-rprx-vision",
		Network: "tcp",
		TLS:     true,
		SNI:     "www.microsoft.com",
		Reality: &domain.RealityOptions{
			PublicKey: "abcdefghijklmnopqrstuvwxyz0123456789ABCDEF=",
			ShortID:   "0123456789abcdef",
			SpiderX:   "/",
		},
		ClientFingerprint: "chrome",
	}
	if err := vlessNode.SetProtocolOptions(vlessOpts); err != nil {
		t.Fatalf("SetProtocolOptions(vless) failed: %v", err)
	}
	if err := vlessNode.Validate(); err != nil {
		t.Fatalf("vlessNode.Validate() failed: %v", err)
	}
	gotVLESS, err := vlessNode.GetVLESSOptions()
	if err != nil || gotVLESS.Reality == nil || gotVLESS.Reality.ShortID != "0123456789abcdef" {
		t.Fatalf("GetVLESSOptions() mismatch: %+v, err: %v", gotVLESS, err)
	}

	// 4. Trojan
	trojanNode := &domain.Node{
		LogicalID: "node-trojan-1",
		Name:      "Trojan-SG-01",
		Protocol:  domain.ProtocolTrojan,
		Server:    "sg.example.com",
		Port:      443,
	}
	trojanOpts := domain.TrojanOptions{
		Password:       "trojanpassword",
		SNI:            "sg.example.com",
		ALPN:           []string{"h2", "http/1.1"},
		SkipCertVerify: true,
		Network:        "grpc",
		GRPCOptions: &domain.GRPCOptions{
			ServiceName: "TrojanService",
		},
	}
	if err := trojanNode.SetProtocolOptions(trojanOpts); err != nil {
		t.Fatalf("SetProtocolOptions(trojan) failed: %v", err)
	}
	if err := trojanNode.Validate(); err != nil {
		t.Fatalf("trojanNode.Validate() failed: %v", err)
	}

	// 5. Hysteria2
	hy2Node := &domain.Node{
		LogicalID: "node-hy2-1",
		Name:      "Hy2-KR-01",
		Protocol:  domain.ProtocolHysteria2,
		Server:    "kr.example.com",
		Port:      8443,
	}
	hy2Opts := domain.Hysteria2Options{
		Password:       "hy2password",
		Ports:          "20000-40000",
		SNI:            "kr.example.com",
		SkipCertVerify: true,
		ALPN:           []string{"h3"},
		Obfs:           "salamander",
		ObfsPassword:   "obfspass",
		Up:             "100 Mbps",
		Down:           "500 Mbps",
	}
	if err := hy2Node.SetProtocolOptions(hy2Opts); err != nil {
		t.Fatalf("SetProtocolOptions(hysteria2) failed: %v", err)
	}
	if err := hy2Node.Validate(); err != nil {
		t.Fatalf("hy2Node.Validate() failed: %v", err)
	}
	gotHy2, err := hy2Node.GetHysteria2Options()
	if err != nil || gotHy2.Obfs != "salamander" || gotHy2.Ports != "20000-40000" {
		t.Fatalf("GetHysteria2Options() mismatch: %+v, err: %v", gotHy2, err)
	}

	// 6. TUIC
	tuicNode := &domain.Node{
		LogicalID: "node-tuic-1",
		Name:      "TUIC-TW-01",
		Protocol:  domain.ProtocolTUIC,
		Server:    "tw.example.com",
		Port:      8443,
	}
	tuicOpts := domain.TUICOptions{
		UUID:                 "33445566-7788-99aa-bbcc-ddeeff001122",
		Password:             "tuicpass",
		CongestionController: "bbr",
		UDPRelayMode:         "native",
		SNI:                  "tw.example.com",
		ALPN:                 []string{"h3"},
	}
	if err := tuicNode.SetProtocolOptions(tuicOpts); err != nil {
		t.Fatalf("SetProtocolOptions(tuic) failed: %v", err)
	}
	if err := tuicNode.Validate(); err != nil {
		t.Fatalf("tuicNode.Validate() failed: %v", err)
	}
	gotTUIC, err := tuicNode.GetTUICOptions()
	if err != nil || gotTUIC.CongestionController != "bbr" || gotTUIC.UDPRelayMode != "native" {
		t.Fatalf("GetTUICOptions() mismatch: %+v, err: %v", gotTUIC, err)
	}
}

func TestNode_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		node    domain.Node
		wantErr bool
	}{
		{
			name: "missing name",
			node: domain.Node{
				LogicalID: "test-1",
				Protocol:  domain.ProtocolShadowsocks,
				Server:    "1.1.1.1",
				Port:      8388,
			},
			wantErr: true,
		},
		{
			name: "missing server",
			node: domain.Node{
				Name:      "Test",
				LogicalID: "test-2",
				Protocol:  domain.ProtocolShadowsocks,
				Server:    "",
				Port:      8388,
			},
			wantErr: true,
		},
		{
			name: "invalid port 0",
			node: domain.Node{
				Name:      "Test",
				LogicalID: "test-3",
				Protocol:  domain.ProtocolShadowsocks,
				Server:    "1.1.1.1",
				Port:      0,
			},
			wantErr: true,
		},
		{
			name: "invalid port 70000",
			node: domain.Node{
				Name:      "Test",
				LogicalID: "test-4",
				Protocol:  domain.ProtocolShadowsocks,
				Server:    "1.1.1.1",
				Port:      70000,
			},
			wantErr: true,
		},
		{
			name: "unknown protocol",
			node: domain.Node{
				Name:      "Test",
				LogicalID: "test-5",
				Protocol:  "invalid-proto",
				Server:    "1.1.1.1",
				Port:      8388,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.node.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Node.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNode_FingerprintAndSanitization(t *testing.T) {
	node := &domain.Node{
		ID:        99,
		LogicalID: "logical-id-xyz",
		Name:      "Sanitize Test",
		Protocol:  domain.ProtocolVLESS,
		Server:    "reality.example.com",
		Port:      443,
		NormalizedPayload: map[string]any{
			"uuid":        "test-uuid",
			"server":      "reality.example.com",
			"port":        443,
			"type":        "vless",
			"sub_id":      12,
			"source":      "sub1",
			"internal_id": 99,
		},
		LifecycleState: domain.LifecycleActive,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	fp := node.ComputeFingerprint()
	if fp == "" || len(fp) < 10 {
		t.Fatalf("ComputeFingerprint() returned invalid fingerprint: %q", fp)
	}

	sanitized := node.SanitizeForExport()
	for _, forbidden := range []string{"sub_id", "source", "internal_id", "lifecycle_state"} {
		if _, exists := sanitized[forbidden]; exists {
			t.Errorf("SanitizeForExport() still contains internal field %q", forbidden)
		}
	}
	if sanitized["server"] != "reality.example.com" || sanitized["port"] != 443 {
		t.Errorf("SanitizeForExport() lost connection fields: %+v", sanitized)
	}
}

func TestNode_Serialization(t *testing.T) {
	node := &domain.Node{
		ID:                 10,
		LogicalID:          "uuid-1234",
		Name:               "Test Node",
		Protocol:           domain.ProtocolTrojan,
		Server:             "trojan.test",
		Port:               443,
		PayloadFingerprint: "v1:abcdef123456",
		LifecycleState:     domain.LifecycleActive,
		NormalizedPayload: map[string]any{
			"password": "pass",
			"sni":      "trojan.test",
		},
	}

	// JSON test
	data, err := json.Marshal(node)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var unmarshaled domain.Node
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if unmarshaled.Name != node.Name || unmarshaled.Protocol != node.Protocol {
		t.Errorf("JSON mismatch: %+v vs %+v", unmarshaled, node)
	}

	// YAML test
	yamlBytes, err := yaml.Marshal(node)
	if err != nil {
		t.Fatalf("yaml.Marshal failed: %v", err)
	}
	var yamlNode domain.Node
	if err := yaml.Unmarshal(yamlBytes, &yamlNode); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}
	if yamlNode.Server != node.Server || yamlNode.Port != node.Port {
		t.Errorf("YAML mismatch: %+v vs %+v", yamlNode, node)
	}
}
