package parser_test

import (
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/parser"
)

func TestExtractWithCredentialsYAML(t *testing.T) {
	content := readFixture(t, "testdata/clash.yaml")
	res, err := parser.ExtractWithCredentials(content)
	if err != nil {
		t.Fatalf("ExtractWithCredentials error: %v", err)
	}
	if len(res.Items) != 7 {
		t.Fatalf("expected 7 items, got %d", len(res.Items))
	}

	for _, item := range res.Items {
		switch item.Normalized.Node.Protocol {
		case domain.ProtocolSS:
			if item.Credentials.Password != "ss-password" || item.Credentials.Method != "aes-256-gcm" {
				t.Errorf("SS credentials mismatch: %+v", item.Credentials)
			}
		case domain.ProtocolVMess:
			if item.Credentials.UUID != "11111111-1111-1111-1111-111111111111" {
				t.Errorf("VMess UUID mismatch: %+v", item.Credentials)
			}
		case domain.ProtocolVLESS:
			if item.Credentials.UUID != "22222222-2222-2222-2222-222222222222" {
				t.Errorf("VLESS UUID mismatch: %+v", item.Credentials)
			}
		case domain.ProtocolTrojan:
			if item.Credentials.Password != "trojan-password" {
				t.Errorf("Trojan password mismatch: %+v", item.Credentials)
			}
		case domain.ProtocolHysteria2:
			if item.Credentials.Password != "hy2-password" {
				t.Errorf("Hysteria2 password mismatch: %+v", item.Credentials)
			}
		case domain.ProtocolWireGuard:
			if item.Credentials.PrivateKey != "wireguard-private-key" || item.Credentials.PublicKey != "wireguard-public-key" ||
				!reflect.DeepEqual(item.Credentials.LocalAddress, []string{"10.0.0.2/32"}) {
				t.Errorf("WireGuard mismatch: %+v", item.Credentials)
			}
		case domain.ProtocolTUIC:
			if item.Credentials.UUID != "33333333-3333-3333-3333-333333333333" || item.Credentials.Password != "tuic-password" {
				t.Errorf("TUIC mismatch: %+v", item.Credentials)
			}
		}
	}
}

func TestExtractWithCredentialsURLLines(t *testing.T) {
	content := readFixture(t, "testdata/subscription.txt")
	res, err := parser.ExtractWithCredentials(content)
	if err != nil {
		t.Fatalf("ExtractWithCredentials error: %v", err)
	}
	if len(res.Items) != 7 {
		t.Fatalf("expected 7 items, got %d", len(res.Items))
	}
	if res.Rejected != 1 {
		t.Fatalf("expected 1 rejected (unsupported://), got %d", res.Rejected)
	}

	for _, item := range res.Items {
		switch item.Normalized.Node.Protocol {
		case domain.ProtocolSS:
			if item.Credentials.Password != "ss-password" || item.Credentials.Method != "aes-256-gcm" {
				t.Errorf("SS credentials mismatch: %+v", item.Credentials)
			}
		case domain.ProtocolVMess:
			if item.Credentials.UUID != "11111111-1111-1111-1111-111111111111" {
				t.Errorf("VMess UUID mismatch: %+v", item.Credentials)
			}
		case domain.ProtocolVLESS:
			if item.Credentials.UUID != "22222222-2222-2222-2222-222222222222" {
				t.Errorf("VLESS UUID mismatch: %+v", item.Credentials)
			}
		case domain.ProtocolTrojan:
			if item.Credentials.Password != "trojan-password" {
				t.Errorf("Trojan password mismatch: %+v", item.Credentials)
			}
		case domain.ProtocolHysteria2:
			if item.Credentials.Password != "hy2-password" {
				t.Errorf("Hysteria2 password mismatch: %+v", item.Credentials)
			}
		case domain.ProtocolWireGuard:
			if item.Credentials.PrivateKey != "wireguard-private-key" || item.Credentials.PublicKey != "wireguard-public-key" ||
				!reflect.DeepEqual(item.Credentials.LocalAddress, []string{"10.0.0.2/32"}) {
				t.Errorf("WireGuard mismatch: %+v", item.Credentials)
			}
		case domain.ProtocolTUIC:
			if item.Credentials.UUID != "33333333-3333-3333-3333-333333333333" || item.Credentials.Password != "tuic-password" {
				t.Errorf("TUIC mismatch: %+v", item.Credentials)
			}
		}
	}
}

func TestExtractWithCredentials_AllSevenProtocolsFullFieldsAndDualFormatParity(t *testing.T) {
	yamlInput := []byte(`proxies:
  - name: Full SS
    type: ss
    server: ss-full.example.com
    port: 8388
    cipher: 2022-blake3-aes-128-gcm
    password: ss-full-password
  - name: Full VMess
    type: vmess
    server: vmess-full.example.com
    port: 443
    uuid: 11111111-1111-1111-1111-111111111111
    alterId: 2
    cipher: auto
    network: ws
    tls: true
    servername: vmess-cdn.example.com
    ws-opts:
      path: /vmess-ws
      headers:
        Host: vmess-cdn.example.com
  - name: Full VLESS Reality
    type: vless
    server: vless-reality.example.com
    port: 443
    uuid: 22222222-2222-2222-2222-222222222222
    network: tcp
    tls: true
    servername: reality.example.com
    flow: xtls-rprx-vision
    client-fingerprint: chrome
    reality-opts:
      public-key: reality-pbk-value
      short-id: a1b2c3d4
  - name: Full Trojan
    type: trojan
    server: trojan-full.example.com
    port: 443
    password: trojan-full-password
    sni: trojan-full.example.com
    skip-cert-verify: true
    alpn:
      - h2
      - http/1.1
  - name: Full Hysteria2
    type: hysteria2
    server: hy2-full.example.com
    port: 8443
    password: hy2-full-password
    sni: hy2-full.example.com
    skip-cert-verify: true
    up: 100 Mbps
    down: 500 Mbps
    obfs: salamander
    obfs-password: hy2-obfs-secret-value
    ports: 20000-30000
  - name: Full WireGuard
    type: wireguard
    server: wg-full.example.com
    port: 51820
    ip: 10.0.0.2/32
    ipv6: fd00::2/128
    private-key: wg-priv-key-full
    public-key: wg-pub-key-full
    pre-shared-key: wg-psk-full
    reserved: [12, 34, 56]
    mtu: 1420
    dns:
      - 1.1.1.1
      - 8.8.8.8
  - name: Full TUIC
    type: tuic
    server: tuic-full.example.com
    port: 443
    uuid: 33333333-3333-3333-3333-333333333333
    password: tuic-full-password
    congestion-controller: bbr
    udp-relay-mode: native
    alpn:
      - h3
      - h3-29
    sni: tuic-full.example.com
    disable-sni: true
    skip-cert-verify: true
`)

	vmessJSON := `{"v":"2","ps":"Full VMess","add":"vmess-full.example.com","port":"443","id":"11111111-1111-1111-1111-111111111111","aid":"2","scy":"auto","net":"ws","tls":"tls","sni":"vmess-cdn.example.com","host":"vmess-cdn.example.com","path":"/vmess-ws"}`
	ssUser := base64.RawURLEncoding.EncodeToString([]byte("2022-blake3-aes-128-gcm:ss-full-password"))

	uriInput := []byte(strings.Join([]string{
		"ss://" + ssUser + "@ss-full.example.com:8388#Full%20SS",
		"vmess://" + base64.StdEncoding.EncodeToString([]byte(vmessJSON)),
		"vless://22222222-2222-2222-2222-222222222222@vless-reality.example.com:443?type=tcp&security=reality&sni=reality.example.com&flow=xtls-rprx-vision&fp=chrome&pbk=reality-pbk-value&sid=a1b2c3d4#Full%20VLESS%20Reality",
		"trojan://trojan-full-password@trojan-full.example.com:443?sni=trojan-full.example.com&allowInsecure=1&alpn=h2,http%2F1.1#Full%20Trojan",
		"hysteria2://hy2-full-password@hy2-full.example.com:8443?sni=hy2-full.example.com&insecure=1&up=100%20Mbps&down=500%20Mbps&obfs=salamander&obfs-password=hy2-obfs-secret-value&mport=20000-30000#Full%20Hysteria2",
		"wireguard://wg-priv-key-full@wg-full.example.com:51820?public_key=wg-pub-key-full&pre_shared_key=wg-psk-full&local_address=10.0.0.2%2F32,fd00%3A%3A2%2F128&reserved=12,34,56&mtu=1420&dns=1.1.1.1,8.8.8.8#Full%20WireGuard",
		"tuic://33333333-3333-3333-3333-333333333333:tuic-full-password@tuic-full.example.com:443?congestion_control=bbr&udp_relay_mode=native&alpn=h3,h3-29&sni=tuic-full.example.com&disable_sni=true&allowInsecure=true#Full%20TUIC",
	}, "\n"))

	yamlRes, err := parser.ExtractWithCredentials(yamlInput)
	if err != nil {
		t.Fatalf("ExtractWithCredentials(YAML) failed: %v", err)
	}
	if yamlRes.Rejected != 0 || len(yamlRes.Items) != 7 {
		t.Fatalf("YAML expected 7 items and 0 rejected, got %d items and %d rejected", len(yamlRes.Items), yamlRes.Rejected)
	}

	uriRes, err := parser.ExtractWithCredentials(uriInput)
	if err != nil {
		t.Fatalf("ExtractWithCredentials(URI) failed: %v", err)
	}
	if uriRes.Rejected != 0 || len(uriRes.Items) != 7 {
		t.Fatalf("URI expected 7 items and 0 rejected, got %d items and %d rejected", len(uriRes.Items), uriRes.Rejected)
	}

	assertFullProtocolItem := func(t *testing.T, label string, items []parser.ParsedNodeWithCredentials) map[domain.Protocol]parser.ParsedNodeWithCredentials {
		t.Helper()
		byProto := make(map[domain.Protocol]parser.ParsedNodeWithCredentials, len(items))
		for _, item := range items {
			byProto[item.Normalized.Node.Protocol] = item
		}

		// 1. SS
		ss := byProto[domain.ProtocolSS]
		if ss.Credentials.Method != "2022-blake3-aes-128-gcm" || ss.Credentials.Password != "ss-full-password" {
			t.Errorf("%s SS credentials mismatch: %+v", label, ss.Credentials)
		}

		// 2. VMess
		vm := byProto[domain.ProtocolVMess]
		if vm.Credentials.UUID != "11111111-1111-1111-1111-111111111111" || vm.Credentials.AlterID != 2 || vm.Credentials.Method != "auto" {
			t.Errorf("%s VMess credentials mismatch: %+v", label, vm.Credentials)
		}
		if vm.Credentials.Transport["network"] != "ws" || vm.Credentials.Transport["path"] != "/vmess-ws" ||
			vm.Credentials.Transport["host"] != "vmess-cdn.example.com" || vm.Credentials.Transport["sni"] != "vmess-cdn.example.com" ||
			vm.Credentials.Transport["tls"] != "true" {
			t.Errorf("%s VMess transport mismatch: %+v", label, vm.Credentials.Transport)
		}

		// 3. VLESS Reality
		vl := byProto[domain.ProtocolVLESS]
		if vl.Credentials.UUID != "22222222-2222-2222-2222-222222222222" {
			t.Errorf("%s VLESS UUID mismatch: %+v", label, vl.Credentials)
		}
		if vl.Credentials.Transport["pbk"] != "reality-pbk-value" ||
			vl.Credentials.Transport["sid"] != "a1b2c3d4" ||
			vl.Credentials.Transport["fp"] != "chrome" ||
			vl.Credentials.Transport["flow"] != "xtls-rprx-vision" ||
			vl.Credentials.Transport["sni"] != "reality.example.com" ||
			vl.Credentials.Transport["tls"] != "true" {
			t.Errorf("%s VLESS Reality transport mismatch: %+v", label, vl.Credentials.Transport)
		}

		// 4. Trojan
		tr := byProto[domain.ProtocolTrojan]
		if tr.Credentials.Password != "trojan-full-password" {
			t.Errorf("%s Trojan password mismatch: %+v", label, tr.Credentials)
		}
		if tr.Credentials.Transport["sni"] != "trojan-full.example.com" ||
			tr.Credentials.Transport["skip_cert_verify"] != "true" ||
			tr.Credentials.Transport["alpn"] != "h2,http/1.1" {
			t.Errorf("%s Trojan transport mismatch: %+v", label, tr.Credentials.Transport)
		}

		// 5. Hysteria2
		hy2 := byProto[domain.ProtocolHysteria2]
		if hy2.Credentials.Password != "hy2-full-password" {
			t.Errorf("%s Hysteria2 password mismatch: %+v", label, hy2.Credentials)
		}
		if hy2.Credentials.Transport["up"] != "100 Mbps" ||
			hy2.Credentials.Transport["down"] != "500 Mbps" ||
			hy2.Credentials.Transport["obfs"] != "salamander" ||
			hy2.Credentials.Transport["obfs-password"] != "hy2-obfs-secret-value" ||
			hy2.Credentials.Transport["server_ports"] != "20000-30000" ||
			hy2.Credentials.Transport["sni"] != "hy2-full.example.com" ||
			hy2.Credentials.Transport["skip_cert_verify"] != "true" {
			t.Errorf("%s Hysteria2 transport mismatch: %+v", label, hy2.Credentials.Transport)
		}
		if _, leaked := hy2.Normalized.Transport["obfs-password"]; leaked {
			t.Errorf("%s Hysteria2 normalized transport leaked obfs-password: %+v", label, hy2.Normalized.Transport)
		}

		// 6. WireGuard
		wg := byProto[domain.ProtocolWireGuard]
		wantAddrs := []string{"10.0.0.2/32", "fd00::2/128"}
		wantReserved := []uint8{12, 34, 56}
		wantDNS := []string{"1.1.1.1", "8.8.8.8"}
		if wg.Credentials.PrivateKey != "wg-priv-key-full" ||
			wg.Credentials.PublicKey != "wg-pub-key-full" ||
			wg.Credentials.PreSharedKey != "wg-psk-full" ||
			wg.Credentials.EffectivePreSharedKey() != "wg-psk-full" ||
			!reflect.DeepEqual(wg.Credentials.LocalAddress, wantAddrs) ||
			!reflect.DeepEqual(wg.Credentials.Reserved, wantReserved) ||
			wg.Credentials.MTU != 1420 ||
			!reflect.DeepEqual(wg.Credentials.DNS, wantDNS) {
			t.Errorf("%s WireGuard credentials mismatch: %+v", label, wg.Credentials)
		}

		// 7. TUIC
		tuic := byProto[domain.ProtocolTUIC]
		wantALPN := []string{"h3", "h3-29"}
		if tuic.Credentials.UUID != "33333333-3333-3333-3333-333333333333" ||
			tuic.Credentials.Password != "tuic-full-password" ||
			tuic.Credentials.CongestionControl != "bbr" ||
			tuic.Credentials.UDPRelayMode != "native" ||
			!reflect.DeepEqual(tuic.Credentials.ALPN, wantALPN) ||
			tuic.Credentials.SNI != "tuic-full.example.com" ||
			!tuic.Credentials.DisableSNI ||
			tuic.Credentials.Transport["disable_sni"] != "true" ||
			tuic.Credentials.Transport["skip_cert_verify"] != "true" {
			t.Errorf("%s TUIC credentials mismatch: %+v", label, tuic.Credentials)
		}

		return byProto
	}

	yamlByProto := assertFullProtocolItem(t, "YAML", yamlRes.Items)
	uriByProto := assertFullProtocolItem(t, "URI", uriRes.Items)

	// Verify LogicalID parity across YAML and URI for all 7 protocols
	for proto, yamlItem := range yamlByProto {
		uriItem, ok := uriByProto[proto]
		if !ok {
			t.Fatalf("missing protocol %s in URI result", proto)
		}
		if yamlItem.Normalized.Node.LogicalID != uriItem.Normalized.Node.LogicalID {
			t.Errorf("%s logical ID mismatch between YAML (%s) and URI (%s)",
				proto, yamlItem.Normalized.Node.LogicalID, uriItem.Normalized.Node.LogicalID)
		}
	}

	// Verify Parse() produces complete domain.Node with Server, Port, and Credentials
	parsedYAML, err := parser.Parse(yamlInput)
	if err != nil {
		t.Fatalf("Parse(YAML) failed: %v", err)
	}
	if len(parsedYAML.Nodes) != 7 || parsedYAML.Rejected != 0 {
		t.Fatalf("Parse(YAML) expected 7 nodes and 0 rejected, got %d and %d", len(parsedYAML.Nodes), parsedYAML.Rejected)
	}
	encoded, err := json.Marshal(parsedYAML)
	if err != nil {
		t.Fatalf("json.Marshal(parsedYAML) failed: %v", err)
	}
	for _, secret := range []string{
		"ss-full-password",
		"11111111-1111-1111-1111-111111111111",
		"22222222-2222-2222-2222-222222222222",
		"trojan-full-password",
		"hy2-full-password",
		"hy2-obfs-secret-value",
		"wg-priv-key-full",
		"wg-pub-key-full",
		"wg-psk-full",
		"33333333-3333-3333-3333-333333333333",
		"tuic-full-password",
	} {
		if !strings.Contains(string(encoded), secret) {
			t.Errorf("Parse() output missing expected plaintext credential %q: %s", secret, string(encoded))
		}
	}
}

func TestExtractWithCredentials_WireGuardPeersAndBase64Reserved(t *testing.T) {
	// Test YAML peers[0] extraction and Base64 reserved in URI
	reservedB64 := base64.StdEncoding.EncodeToString([]byte{7, 8, 9})
	yamlInput := []byte(`proxies:
  - name: WG Peers Node
    type: wireguard
    server: wg-peers.example.com
    port: 51820
    local-address:
      - 10.10.0.2
    private-key: wg-peer-priv
    mtu: 1380
    dns: 1.1.1.1, 1.0.0.1
    peers:
      - public-key: wg-peer-pub
        pre-shared-key: wg-peer-psk
        reserved: [7, 8, 9]
`)
	uriInput := []byte("wireguard://wg-peer-priv@wg-peers.example.com:51820?public_key=wg-peer-pub&psk=wg-peer-psk&address=10.10.0.2&reserved=" + reservedB64 + "&mtu=1380&dns=1.1.1.1,1.0.0.1#WG%20Peers%20Node")

	yamlRes, err := parser.ExtractWithCredentials(yamlInput)
	if err != nil {
		t.Fatalf("ExtractWithCredentials(YAML) failed: %v", err)
	}
	uriRes, err := parser.ExtractWithCredentials(uriInput)
	if err != nil {
		t.Fatalf("ExtractWithCredentials(URI) failed: %v", err)
	}

	for label, item := range map[string]parser.ParsedNodeWithCredentials{
		"YAML": yamlRes.Items[0],
		"URI":  uriRes.Items[0],
	} {
		if item.Credentials.PrivateKey != "wg-peer-priv" ||
			item.Credentials.PublicKey != "wg-peer-pub" ||
			item.Credentials.EffectivePreSharedKey() != "wg-peer-psk" ||
			!reflect.DeepEqual(item.Credentials.LocalAddress, []string{"10.10.0.2/32"}) ||
			!reflect.DeepEqual(item.Credentials.Reserved, []uint8{7, 8, 9}) ||
			item.Credentials.MTU != 1380 ||
			!reflect.DeepEqual(item.Credentials.DNS, []string{"1.1.1.1", "1.0.0.1"}) {
			t.Errorf("%s WG peers/reserved mismatch: %+v", label, item.Credentials)
		}
	}
}

func TestExtractWithCredentials_MissingRequiredFieldsAndInvalidValuesRejected(t *testing.T) {
	validSSYAML := `  - name: Valid Anchor SS
    type: ss
    server: anchor.example.com
    port: 8388
    cipher: aes-256-gcm
    password: anchor-password`

	validSSURI := "ss://YWVzLTI1Ni1nY206YW5jaG9yLXBhc3N3b3Jk@anchor.example.com:8388#Valid%20Anchor%20SS"

	vmessNoUUID := base64.StdEncoding.EncodeToString([]byte(`{"v":"2","ps":"Bad VMess","add":"vmess.example.com","port":"443","id":""}`))

	cases := []struct {
		name    string
		badYAML string
		badURI  string
	}{
		{
			name: "SS missing password",
			badYAML: `  - name: Bad SS
    type: ss
    server: ss.example.com
    port: 8388
    cipher: aes-256-gcm`,
			badURI: "ss://aes-256-gcm@ss.example.com:8388#Bad%20SS",
		},
		{
			name: "VMess missing uuid",
			badYAML: `  - name: Bad VMess
    type: vmess
    server: vmess.example.com
    port: 443
    cipher: auto`,
			badURI: "vmess://" + vmessNoUUID,
		},
		{
			name: "VLESS missing uuid",
			badYAML: `  - name: Bad VLESS
    type: vless
    server: vless.example.com
    port: 443`,
			badURI: "vless://vless.example.com:443?security=tls#Bad%20VLESS",
		},
		{
			name: "Trojan missing password",
			badYAML: `  - name: Bad Trojan
    type: trojan
    server: trojan.example.com
    port: 443`,
			badURI: "trojan://trojan.example.com:443?sni=trojan.example.com#Bad%20Trojan",
		},
		{
			name: "Hysteria2 missing password",
			badYAML: `  - name: Bad Hy2
    type: hysteria2
    server: hy2.example.com
    port: 8443`,
			badURI: "hysteria2://hy2.example.com:8443?sni=hy2.example.com#Bad%20Hy2",
		},
		{
			name: "WireGuard missing private_key",
			badYAML: `  - name: Bad WG No PrivKey
    type: wireguard
    server: wg.example.com
    port: 51820
    ip: 10.0.0.2/32
    public-key: wg-pub`,
			badURI: "wireguard://wg.example.com:51820?public_key=wg-pub&address=10.0.0.2%2F32#Bad%20WG",
		},
		{
			name: "WireGuard missing public_key",
			badYAML: `  - name: Bad WG No PubKey
    type: wireguard
    server: wg.example.com
    port: 51820
    ip: 10.0.0.2/32
    private-key: wg-priv`,
			badURI: "wireguard://wg-priv@wg.example.com:51820?address=10.0.0.2%2F32#Bad%20WG",
		},
		{
			name: "WireGuard missing local_address",
			badYAML: `  - name: Bad WG No Address
    type: wireguard
    server: wg.example.com
    port: 51820
    private-key: wg-priv
    public-key: wg-pub`,
			badURI: "wireguard://wg-priv@wg.example.com:51820?public_key=wg-pub#Bad%20WG",
		},
		{
			name: "WireGuard invalid local_address",
			badYAML: `  - name: Bad WG Invalid Address
    type: wireguard
    server: wg.example.com
    port: 51820
    ip: not-an-ip
    private-key: wg-priv
    public-key: wg-pub`,
			badURI: "wireguard://wg-priv@wg.example.com:51820?public_key=wg-pub&address=not-an-ip#Bad%20WG",
		},
		{
			name: "WireGuard invalid reserved length",
			badYAML: `  - name: Bad WG Reserved
    type: wireguard
    server: wg.example.com
    port: 51820
    ip: 10.0.0.2/32
    private-key: wg-priv
    public-key: wg-pub
    reserved: [1, 2]`,
			badURI: "wireguard://wg-priv@wg.example.com:51820?public_key=wg-pub&address=10.0.0.2%2F32&reserved=1,2#Bad%20WG",
		},
		{
			name: "WireGuard invalid mtu",
			badYAML: `  - name: Bad WG MTU
    type: wireguard
    server: wg.example.com
    port: 51820
    ip: 10.0.0.2/32
    private-key: wg-priv
    public-key: wg-pub
    mtu: -10`,
			badURI: "wireguard://wg-priv@wg.example.com:51820?public_key=wg-pub&address=10.0.0.2%2F32&mtu=0#Bad%20WG",
		},
		{
			name: "TUIC missing uuid",
			badYAML: `  - name: Bad TUIC No UUID
    type: tuic
    server: tuic.example.com
    port: 443
    password: tuic-pass`,
			badURI: "tuic://:tuic-pass@tuic.example.com:443#Bad%20TUIC",
		},
		{
			name: "TUIC missing password",
			badYAML: `  - name: Bad TUIC No Password
    type: tuic
    server: tuic.example.com
    port: 443
    uuid: 33333333-3333-3333-3333-333333333333`,
			badURI: "tuic://33333333-3333-3333-3333-333333333333@tuic.example.com:443#Bad%20TUIC",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// 1. Standalone invalid YAML entry must fail with error
			onlyBadYAML := []byte("proxies:\n" + tc.badYAML + "\n")
			if _, err := parser.ExtractWithCredentials(onlyBadYAML); err == nil {
				t.Errorf("expected standalone invalid YAML (%s) to fail, got nil", tc.name)
			}
			if _, err := parser.Parse(onlyBadYAML); err == nil {
				t.Errorf("expected standalone Parse(YAML) (%s) to fail, got nil", tc.name)
			}

			// 2. Mixed YAML entry must reject the invalid node and keep only the valid anchor
			mixedYAML := []byte("proxies:\n" + tc.badYAML + "\n" + validSSYAML + "\n")
			yamlRes, err := parser.ExtractWithCredentials(mixedYAML)
			if err != nil {
				t.Fatalf("unexpected error on mixed YAML (%s): %v", tc.name, err)
			}
			if yamlRes.Rejected != 1 || len(yamlRes.Items) != 1 {
				t.Errorf("mixed YAML (%s) expected 1 item and 1 rejected, got %d items and %d rejected",
					tc.name, len(yamlRes.Items), yamlRes.Rejected)
			}

			// 3. Standalone invalid URI entry must fail with error
			if _, err := parser.ExtractWithCredentials([]byte(tc.badURI)); err == nil {
				t.Errorf("expected standalone invalid URI (%s) to fail, got nil", tc.name)
			}
			if _, err := parser.Parse([]byte(tc.badURI)); err == nil {
				t.Errorf("expected standalone Parse(URI) (%s) to fail, got nil", tc.name)
			}

			// 4. Mixed URI entry must reject the invalid node and keep only the valid anchor
			mixedURI := []byte(tc.badURI + "\n" + validSSURI + "\n")
			uriRes, err := parser.ExtractWithCredentials(mixedURI)
			if err != nil {
				t.Fatalf("unexpected error on mixed URI (%s): %v", tc.name, err)
			}
			if uriRes.Rejected != 1 || len(uriRes.Items) != 1 {
				t.Errorf("mixed URI (%s) expected 1 item and 1 rejected, got %d items and %d rejected",
					tc.name, len(uriRes.Items), uriRes.Rejected)
			}
		})
	}
}

func TestExtractWithCredentials_HTTP_Socks5_AnyTLS_YAML_and_URI(t *testing.T) {
	// Synthetic fixtures
	yamlContent := `proxies:
  - name: "Synthetic-HTTP"
    type: http
    server: "http.synthetic.test"
    port: 8080
    username: "synthetic-user"
    password: "synthetic-pass"
    tls: true
    sni: "http.synthetic.test"
    skip-cert-verify: true
    headers:
      X-Synthetic: "header-val"
  - name: "Synthetic-Socks5"
    type: socks5
    server: "socks5.synthetic.test"
    port: 1080
    username: "synthetic-socks-user"
    password: "synthetic-socks-pass"
    tls: true
    udp: true
    sni: "socks5.synthetic.test"
    skip-cert-verify: true
  - name: "Synthetic-AnyTLS"
    type: anytls
    server: "anytls.synthetic.test"
    port: 443
    password: "synthetic-anytls-pass"
    sni: "anytls.synthetic.test"
    alpn:
      - h2
      - http/1.1
    client-fingerprint: "chrome"
    udp: true
    skip-cert-verify: true
    idle-session-check-interval: 30
    idle-session-timeout: 60
    min-idle-session: 2
    disable-reuse: true
    ech-opts:
      enable: true
      query-server-name: "cloudflarechallenge.com"
`

	res, err := parser.ExtractWithCredentials([]byte(yamlContent))
	if err != nil {
		t.Fatalf("ExtractWithCredentials YAML error: %v", err)
	}
	if len(res.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(res.Items))
	}

	httpItem := res.Items[0]
	if httpItem.Normalized.Node.Protocol != domain.ProtocolHTTP {
		t.Errorf("expected HTTP protocol, got %s", httpItem.Normalized.Node.Protocol)
	}
	if httpItem.Credentials.Username != "synthetic-user" || httpItem.Credentials.Password != "synthetic-pass" {
		t.Errorf("HTTP credentials mismatch: user=%s pass=%s", httpItem.Credentials.Username, httpItem.Credentials.Password)
	}
	if httpItem.Normalized.Transport["tls"] != "true" || httpItem.Normalized.Transport["skip_cert_verify"] != "true" {
		t.Errorf("HTTP transport mismatch: %+v", httpItem.Normalized.Transport)
	}

	socksItem := res.Items[1]
	if socksItem.Normalized.Node.Protocol != domain.ProtocolSocks5 {
		t.Errorf("expected Socks5 protocol, got %s", socksItem.Normalized.Node.Protocol)
	}
	if socksItem.Normalized.Transport["udp"] != "true" {
		t.Errorf("expected socks5 udp=true, got: %+v", socksItem.Normalized.Transport)
	}

	anytlsItem := res.Items[2]
	if anytlsItem.Normalized.Node.Protocol != domain.ProtocolAnyTLS {
		t.Errorf("expected AnyTLS protocol, got %s", anytlsItem.Normalized.Node.Protocol)
	}
	if anytlsItem.Credentials.Password != "synthetic-anytls-pass" {
		t.Errorf("AnyTLS password mismatch: %s", anytlsItem.Credentials.Password)
	}
	if anytlsItem.Normalized.Transport["fp"] != "chrome" {
		t.Errorf("AnyTLS fp mismatch: %s", anytlsItem.Normalized.Transport["fp"])
	}
	if anytlsItem.Normalized.Transport["ech-opts"] == "" {
		t.Errorf("expected AnyTLS ech-opts in transport")
	}

	// URI lines
	uriContent := strings.Join([]string{
		"http://synthetic-user:synthetic-pass@http.synthetic.test:8080#Synthetic-HTTP-URI",
		"https://synthetic-user:synthetic-pass@https.synthetic.test:8443?sni=https.synthetic.test#Synthetic-HTTPS-URI",
		"socks5://synthetic-socks-user:synthetic-socks-pass@socks5.synthetic.test:1080?udp=true#Synthetic-Socks5-URI",
		"anytls://synthetic-anytls-pass@anytls.synthetic.test:443?sni=anytls.synthetic.test&alpn=h2,http/1.1&udp=true#Synthetic-AnyTLS-URI",
	}, "\n")

	uriRes, err := parser.ExtractWithCredentials([]byte(uriContent))
	if err != nil {
		t.Fatalf("ExtractWithCredentials URI error: %v", err)
	}
	if len(uriRes.Items) != 4 {
		t.Fatalf("expected 4 items from URI, got %d", len(uriRes.Items))
	}
	if uriRes.Items[0].Normalized.Node.Protocol != domain.ProtocolHTTP {
		t.Errorf("expected HTTP, got %s", uriRes.Items[0].Normalized.Node.Protocol)
	}
	if uriRes.Items[1].Normalized.Node.Protocol != domain.ProtocolHTTP || uriRes.Items[1].Normalized.Transport["tls"] != "true" {
		t.Errorf("expected HTTPS with tls=true, got %+v", uriRes.Items[1].Normalized)
	}
	if uriRes.Items[2].Normalized.Node.Protocol != domain.ProtocolSocks5 || uriRes.Items[2].Normalized.Transport["udp"] != "true" {
		t.Errorf("expected SOCKS5 with udp=true, got %+v", uriRes.Items[2].Normalized)
	}
	if uriRes.Items[3].Normalized.Node.Protocol != domain.ProtocolAnyTLS || uriRes.Items[3].Credentials.Password != "synthetic-anytls-pass" {
		t.Errorf("expected AnyTLS with pass, got %+v", uriRes.Items[3].Normalized)
	}
}
