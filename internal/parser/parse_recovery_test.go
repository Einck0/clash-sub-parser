package parser_test

import (
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"clash-sub-parser/internal/parser"
)

func TestParseRecoveryYAMLElementsAndOneBase64Layer(t *testing.T) {
	const valid = "  - {name: valid, type: ss, server: 192.0.2.8, port: 8388, cipher: aes-128-gcm, password: SYNTHETIC_PASSWORD_SENTINEL}\n"
	body := "proxies:\n  - SECRET_SCALAR_SENTINEL\n  - [SECRET_SEQUENCE_SENTINEL]\n" + valid +
		"  - {name: SECRET_NAME_SENTINEL, type: SECRET_PROTOCOL_SENTINEL, server: 192.0.2.1, port: 443}\n" +
		"  - {type: trojan, server: 192.0.2.1, port: 443}\n" +
		"  - {type: wireguard, server: 192.0.2.2, port: 51820, private-key: SECRET_KEY_SENTINEL, public-key: public, ip: SECRET_ADDRESS_SENTINEL}\n" + valid
	for _, wrapped := range []bool{false, true} {
		input := body
		format := "yaml"
		if wrapped {
			input = base64.StdEncoding.EncodeToString([]byte(body))
			format = "base64_yaml"
		}
		res, err := parser.ExtractWithCredentials([]byte(input))
		if err != nil || len(res.Items) != 2 || res.Rejected != 5 || len(res.Diagnostics) != 5 {
			t.Fatalf("wrapped=%t items=%d rejected=%d diagnostics=%d error=%v", wrapped, len(res.Items), res.Rejected, len(res.Diagnostics), err)
		}
		wantEntries := []int{1, 2, 4, 5, 6}
		for i, d := range res.Diagnostics {
			if d.Entry != wantEntries[i] || d.Format != format {
				t.Fatalf("wrong ordinal/format: %+v", d)
			}
		}
		if res.Diagnostics[2].Protocol != "" || res.Diagnostics[3].Field != "password" || res.Diagnostics[4].Field != "local_address" {
			t.Fatalf("unexpected reason classification: %+v", res.Diagnostics)
		}
		if res.Items[0].Credentials.Password != "SYNTHETIC_PASSWORD_SENTINEL" || res.Items[0].Normalized.Node.LogicalID != res.Items[1].Normalized.Node.LogicalID {
			t.Fatal("valid nodes were damaged")
		}
		summary := res.DiagnosticSummary()
		if strings.Contains(summary, "SENTINEL") || strings.Contains(summary, "192.0.2") {
			t.Fatalf("diagnostic leaked subscription content: %s", summary)
		}
		again, _ := parser.ExtractWithCredentials([]byte(input))
		if summary != again.DiagnosticSummary() {
			t.Fatal("diagnostics are not deterministic")
		}
		parsed, err := parser.Parse([]byte(input))
		if err != nil || parsed.Rejected != res.Rejected || !reflect.DeepEqual(parsed.Diagnostics, res.Diagnostics) {
			t.Fatalf("Parse discarded diagnostics: %v", err)
		}
	}
}

func TestParseRecoveryAllFailureCountsAndBoundedDiagnostics(t *testing.T) {
	body := strings.Repeat("wireguard://SECRET_PRIVATE@192.0.2.3:51820?public_key=SECRET_PUBLIC&address=SECRET_INVALID_IP\n", 100)
	res, err := parser.ExtractWithCredentials([]byte(body))
	if err == nil || res.Rejected != 100 || len(res.Items) != 0 || len(res.Diagnostics) != 32 {
		t.Fatalf("all-failure counts: rejected=%d details=%d error=%v", res.Rejected, len(res.Diagnostics), err)
	}
	if len(res.ReasonCounts) != 1 || res.ReasonCounts[0].Reason != "invalid_field" || res.ReasonCounts[0].Count != 100 {
		t.Fatalf("reason totals: %+v", res.ReasonCounts)
	}
	if strings.Contains(res.DiagnosticSummary()+err.Error(), "SECRET") || len(res.DiagnosticSummary()) > 8192 {
		t.Fatal("diagnostics leaked or are unbounded")
	}
	parsed, err := parser.Parse([]byte(body))
	if err == nil || parsed.Rejected != 100 || len(parsed.Diagnostics) != 32 {
		t.Fatalf("Parse all-failure counts lost: rejected=%d error=%v", parsed.Rejected, err)
	}
	for _, input := range []string{"proxies:\n - SECRET\n - null\n", "proxies: [SECRET", "proxies: SECRET", "proxies: []"} {
		if _, err := parser.ExtractWithCredentials([]byte(input)); err == nil || strings.Contains(err.Error(), "SECRET") {
			t.Fatalf("invalid YAML must fail safely: %v", err)
		}
	}
}

func TestParseRecoveryLegacyShadowsocksAndPlugin(t *testing.T) {
	for _, endpoint := range []string{"192.0.2.4:8388", "[2001:db8::4]:8388"} {
		for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
			legacy := "ss://" + encoding.EncodeToString([]byte("aes-128-gcm:secret:colon@password@"+endpoint)) + "#Legacy"
			res, err := parser.ExtractWithCredentials([]byte(legacy))
			if err != nil || len(res.Items) != 1 {
				t.Fatalf("legacy SS parse: %v", err)
			}
			c := res.Items[0].Credentials
			if c.Method != "aes-128-gcm" || c.Password != "secret:colon@password" || res.Items[0].Normalized.Node.DisplayName != "Legacy" {
				t.Fatal("legacy SS authority lost credentials/name")
			}
		}
	}
	userinfo := base64.RawURLEncoding.EncodeToString([]byte("aes-128-gcm:secret:colon"))
	uri := "ss://" + userinfo + "@192.0.2.4:8388?plugin=obfs%3Bmode%3Dtls%3Bhost%3Dcdn.example.org"
	res, err := parser.ExtractWithCredentials([]byte(uri))
	if err != nil || len(res.Items) != 1 {
		t.Fatalf("SIP002 plugin: %v", err)
	}
	transport := res.Items[0].Credentials.Transport
	if transport["plugin"] != "obfs" || transport["plugin-opts"] != "mode=tls;host=cdn.example.org" {
		t.Fatalf("plugin lost: %+v", transport)
	}
	yaml := "proxies:\n - {type: ss, server: 192.0.2.4, port: 8388, cipher: aes-128-gcm, password: secret, plugin: obfs, plugin-opts: {mode: tls, host: cdn.example.org}}"
	res, err = parser.ExtractWithCredentials([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	var opts map[string]any
	if err := json.Unmarshal([]byte(res.Items[0].Credentials.Transport["plugin-opts"]), &opts); err != nil || opts["host"] != "cdn.example.org" || opts["mode"] != "tls" {
		t.Fatalf("YAML plugin options lost: %v", err)
	}
	// A malformed credential must not be recovered by inventing a cipher/password.
	for _, input := range []string{"ss://aes-128-gcm@192.0.2.4:8388", "proxies:\n - {type: ss, server: 192.0.2.4, port: 8388, password: secret}"} {
		res, err := parser.ExtractWithCredentials([]byte(input))
		if err == nil || res.Rejected != 1 || res.Diagnostics[0].Reason != "missing_credential" {
			t.Fatal("missing SS credential accepted")
		}
	}
}

func TestParseRecoveryVMessCipherIsNotTLSSecurity(t *testing.T) {
	for _, tc := range []struct{ fields, method, tls string }{
		{`"security":"tls"`, "auto", "true"},
		{`"security":"tls","scy":"aes-128-gcm"`, "aes-128-gcm", "true"},
		{`"security":"tls","cipher":"chacha20-poly1305"`, "chacha20-poly1305", "true"},
		{`"security":"aes-128-gcm"`, "aes-128-gcm", ""},
	} {
		body := `{"add":"192.0.2.5","port":"443","id":"11111111-1111-4111-8111-111111111111",` + tc.fields + `}`
		res, err := parser.ExtractWithCredentials([]byte("vmess://" + base64.StdEncoding.EncodeToString([]byte(body))))
		if err != nil || len(res.Items) != 1 {
			t.Fatalf("VMess parse: %v", err)
		}
		c := res.Items[0].Credentials
		if c.Method != tc.method || c.Transport["tls"] != tc.tls {
			t.Fatalf("method=%q tls=%q expected %q/%q", c.Method, c.Transport["tls"], tc.method, tc.tls)
		}
	}
}
