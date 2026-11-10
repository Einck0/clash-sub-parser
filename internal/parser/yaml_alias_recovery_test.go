package parser_test

import (
	"encoding/base64"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"clash-sub-parser/internal/parser"
)

func TestYAMLAliasAndMergeRecovery(t *testing.T) {
	entry := "{name: test, type: trojan, server: example.org, port: 443, password: secret}"
	expanded := "proxies: [" + entry + "]\n"
	want, err := parser.ExtractWithCredentials([]byte(expanded))
	if err != nil || len(want.Items) != 1 {
		t.Fatalf("expanded fixture: items=%d err=%v", len(want.Items), err)
	}
	cases := map[string]string{
		"sequence alias":    "nodes: &nodes [" + entry + "]\nproxies: *nodes\n",
		"root merge":        "defaults: &defaults {proxies: [" + entry + "]}\n<<: *defaults\n",
		"explicit override": "defaults: &defaults {proxies: []}\n<<: *defaults\nproxies: [" + entry + "]\n",
	}
	for name, input := range cases {
		for _, wrapped := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/base64=%t", name, wrapped), func(t *testing.T) {
				content := input
				if wrapped {
					content = base64.StdEncoding.EncodeToString([]byte(input))
				}
				got, err := parser.ExtractWithCredentials([]byte(content))
				if err != nil || got.Rejected != 0 || !reflect.DeepEqual(got.Items, want.Items) {
					t.Fatalf("items mismatch or rejection: count=%d rejected=%d err=%v", len(got.Items), got.Rejected, err)
				}
			})
		}
	}
}

func TestYAMLAliasEntryIsolation(t *testing.T) {
	input := `nodes: &nodes
  - {name: good, type: trojan, server: example.org, port: 443, password: secret}
  - {name: bad, type: trojan, server: example.org, port: 443}
proxies: *nodes
`
	got, err := parser.ExtractWithCredentials([]byte(input))
	if err != nil || len(got.Items) != 1 || got.Rejected != 1 {
		t.Fatalf("items=%d rejected=%d err=%v", len(got.Items), got.Rejected, err)
	}
}

func TestYAMLAliasInvalidInputs(t *testing.T) {
	cases := map[string]string{
		"duplicate proxies": "proxies: []\nproxies: []\n",
		"nonsequence alias": "node: &node {password: secret}\nproxies: *node\n",
		"cyclic alias":      "proxies: &nodes [*nodes]\n",
		"malformed":         "proxies: [secret\n",
		"invalid merge":     "defaults: &defaults [secret]\n<<: *defaults\n",
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := parser.ExtractWithCredentials([]byte(input))
			if err == nil || len(got.Items) != 0 {
				t.Fatalf("expected rejection: items=%d err=%v", len(got.Items), err)
			}
			if strings.Contains(fmt.Sprint(got.Diagnostics, err), "secret") {
				t.Fatal("diagnostics leaked input")
			}
		})
	}
}

func TestYAMLAliasExpansionLimit(t *testing.T) {
	var input strings.Builder
	input.WriteString("a: &a [secret, secret, secret, secret, secret, secret, secret, secret, secret, secret]\n")
	previous := "a"
	for _, name := range []string{"b", "c", "d", "e", "f"} {
		fmt.Fprintf(&input, "%s: &%s [", name, name)
		for i := 0; i < 10; i++ {
			fmt.Fprintf(&input, "*%s,", previous)
		}
		input.WriteString("]\n")
		previous = name
	}
	input.WriteString("proxies: [{type: trojan, server: example.org, port: 443, password: secret, extra: *f}]\n")
	got, err := parser.ExtractWithCredentials([]byte(input.String()))
	if err == nil || len(got.Items) != 0 {
		t.Fatalf("expected alias expansion rejection: items=%d err=%v", len(got.Items), err)
	}
	if strings.Contains(fmt.Sprint(got.Diagnostics, err), "secret") {
		t.Fatal("diagnostics leaked input")
	}
}
