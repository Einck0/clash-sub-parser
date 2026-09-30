package singbox_test

import (
	"testing"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/singbox"
	"github.com/sagernet/sing-box/option"
)

func TestVLESSVisionRealityRuntimeMatchesExport(t *testing.T) {
	node := domain.Node{
		LogicalID: "vision", Protocol: domain.ProtocolVLESS, Server: "198.51.100.9", Port: 443,
		Credentials: domain.InboundProtocolCredential{UUID: "11111111-1111-1111-1111-111111111111", Transport: map[string]string{
			"flow": "xtls-rprx-vision", "pbk": "public-key", "sid": "deadbeef", "sni": "example.org", "fp": "chrome",
		}},
	}
	cfg := singbox.NodeConfigFromNode(node)
	runtime, _, err := singbox.BuildOptions(cfg)
	if err != nil {
		t.Fatal(err)
	}
	exported, _, err := singbox.BuildExportNodeOption("vision", node)
	if err != nil {
		t.Fatal(err)
	}
	r, ok := runtime.Outbounds[0].Options.(*option.VLESSOutboundOptions)
	if !ok {
		t.Fatalf("unexpected outbound %T", runtime.Outbounds[0].Options)
	}
	e, ok := exported.Options.(*option.VLESSOutboundOptions)
	if !ok {
		t.Fatalf("unexpected export outbound %T", exported.Options)
	}
	if r.Flow != "xtls-rprx-vision" || r.Flow != e.Flow || r.TLS == nil || e.TLS == nil ||
		r.TLS.Insecure || r.TLS.ServerName != e.TLS.ServerName || r.TLS.Reality == nil || e.TLS.Reality == nil ||
		r.TLS.Reality.PublicKey != e.TLS.Reality.PublicKey || r.TLS.Reality.ShortID != e.TLS.Reality.ShortID ||
		r.TLS.UTLS == nil || r.TLS.UTLS.Fingerprint != e.TLS.UTLS.Fingerprint {
		t.Fatalf("runtime/export differ: runtime=%+v exported=%+v", r, e)
	}
	node.Credentials.Transport["flow"] = "not-a-valid-flow"
	if _, _, err := singbox.BuildOptions(singbox.NodeConfigFromNode(node)); err == nil {
		t.Fatal("accepted invalid vision flow")
	}
	node.Credentials.Transport["flow"] = "xtls-rprx-vision"
	node.Credentials.Transport["skip_cert_verify"] = "true"
	if _, _, err := singbox.BuildOptions(singbox.NodeConfigFromNode(node)); err == nil {
		t.Fatal("accepted insecure TLS")
	}
}
