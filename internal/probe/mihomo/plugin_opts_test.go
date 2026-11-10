package mihomo_test

import (
	"errors"
	"reflect"
	"testing"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/mihomo"
)

func TestShadowsocksPluginOptions(t *testing.T) {
	cases := []struct {
		name      string
		transport map[string]string
		want      map[string]any
		wantError bool
		parse     bool
	}{
		{
			name:      "JSON typed and nested",
			transport: map[string]string{"plugin": "v2ray-plugin", "plugin-opts": `{"mode":"websocket","host":"example.com","tls":true,"headers":{"Host":"example.com"},"count":2}`},
			want:      map[string]any{"mode": "websocket", "host": "example.com", "tls": true, "headers": map[string]any{"Host": "example.com"}, "count": float64(2)},
		},
		{
			name:      "JSON reaches mihomo",
			transport: map[string]string{"plugin": "v2ray-plugin", "plugin_opts": `{"mode":"websocket","host":"example.com","tls":true,"headers":{"Host":"example.com"}}`},
			want:      map[string]any{"mode": "websocket", "host": "example.com", "tls": true, "headers": map[string]any{"Host": "example.com"}},
			parse:     true,
		},
		{
			name:      "legacy semicolon",
			transport: map[string]string{"plugin": "v2ray-plugin", "plugin-opts": "mode=websocket;host=example.com;tls;count=2"},
			want:      map[string]any{"mode": "websocket", "host": "example.com", "tls": true, "count": 2},
		},
		{name: "malformed JSON", transport: map[string]string{"plugin": "v2ray-plugin", "plugin-opts": `{"password":"secret"`}, wantError: true},
		{name: "JSON trailing data", transport: map[string]string{"plugin": "v2ray-plugin", "plugin-opts": `{} trailing`}, wantError: true},
		{name: "JSON array", transport: map[string]string{"plugin": "v2ray-plugin", "plugin-opts": `[]`}, wantError: true},
		{name: "conflicting aliases", transport: map[string]string{"plugin": "v2ray-plugin", "plugin-opts": "tls", "plugin_opts": "host=example.com"}, wantError: true},
		{name: "options without plugin", transport: map[string]string{"plugin-opts": "tls"}, wantError: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node := domain.Node{Protocol: domain.ProtocolSS, Server: "example.com", Port: 443, Credentials: domain.InboundProtocolCredential{Method: "aes-128-gcm", Password: "password", Transport: tc.transport}}
			mapping, err := mihomo.NodeToMapping(node)
			if tc.wantError {
				if !errors.Is(err, domain.ErrClientBuildFailed) {
					t.Fatalf("expected client build error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("NodeToMapping: %v", err)
			}
			if got := mapping["plugin-opts"]; !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("plugin options = %#v, want %#v", got, tc.want)
			}
			if tc.parse {
				proxy, err := mihomo.ParseProxy(node)
				if err != nil || proxy == nil {
					t.Fatalf("ParseProxy: proxy=%v err=%v", proxy, err)
				}
			}
		})
	}
}
