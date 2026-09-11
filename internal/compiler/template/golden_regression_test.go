package template_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"clash-sub-parser/internal/compiler/template"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository"
	"clash-sub-parser/internal/service"
)

var forbiddenTokenSubstrings = []string{
	"admin_token",
	"management_token",
	"source_id",
	"token_hash",
	"leak-prevention-token",
	"secret-source-123",
	"leak-mgmt",
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func assertNoForbiddenTokens(t *testing.T, target string, content []byte) {
	t.Helper()
	text := string(content)
	for _, tok := range forbiddenTokenSubstrings {
		if strings.Contains(text, tok) {
			t.Fatalf("target %s leaked forbidden token: %q", target, tok)
		}
	}
}

func TestGoldenFixtures_SampleBundleParityWithPython(t *testing.T) {
	comp, err := template.NewCompiler()
	if err != nil {
		t.Fatalf("template.NewCompiler failed: %v", err)
	}

	bundle := createSampleBundle()
	ctx := context.Background()
	pythonGoldenDir := filepath.Join("testdata", "python_golden")

	t.Run("Clash_Parity", func(t *testing.T) {
		res, err := comp.Compile(ctx, bundle, domain.TargetClash)
		if err != nil {
			t.Fatalf("Compile Clash failed: %v", err)
		}
		assertNoForbiddenTokens(t, "clash", res.Content)

		pyFile := filepath.Join(pythonGoldenDir, "clash.yaml")
		pyBytes, err := os.ReadFile(pyFile)
		if err != nil {
			t.Skipf("Python golden file %s not found: %v", pyFile, err)
		}

		var goYAML, pyYAML map[string]any
		if err := yaml.Unmarshal(res.Content, &goYAML); err != nil {
			t.Fatalf("Go Clash YAML invalid: %v", err)
		}
		if err := yaml.Unmarshal(pyBytes, &pyYAML); err != nil {
			t.Fatalf("Python Clash YAML invalid: %v", err)
		}

		goProxies, _ := goYAML["proxies"].([]any)
		pyProxies, _ := pyYAML["proxies"].([]any)
		if len(goProxies) != len(pyProxies) {
			t.Errorf("Clash proxies count mismatch: Go has %d, Python has %d", len(goProxies), len(pyProxies))
		}

		goGroups, _ := goYAML["proxy-groups"].([]any)
		pyGroups, _ := pyYAML["proxy-groups"].([]any)
		if len(goGroups) != len(pyGroups) {
			t.Errorf("Clash proxy-groups count mismatch: Go has %d, Python has %d", len(goGroups), len(pyGroups))
		}

		goRules, _ := goYAML["rules"].([]any)
		pyRules, _ := pyYAML["rules"].([]any)
		if len(goRules) != len(pyRules) {
			t.Errorf("Clash rules count mismatch: Go has %d, Python has %d", len(goRules), len(pyRules))
		}
	})

	t.Run("Mihomo_Parity", func(t *testing.T) {
		res, err := comp.Compile(ctx, bundle, domain.TargetMihomo)
		if err != nil {
			t.Fatalf("Compile Mihomo failed: %v", err)
		}
		assertNoForbiddenTokens(t, "mihomo", res.Content)

		pyFile := filepath.Join(pythonGoldenDir, "mihomo.yaml")
		pyBytes, err := os.ReadFile(pyFile)
		if err != nil {
			t.Skipf("Python golden file %s not found: %v", pyFile, err)
		}

		var goYAML, pyYAML map[string]any
		if err := yaml.Unmarshal(res.Content, &goYAML); err != nil {
			t.Fatalf("Go Mihomo YAML invalid: %v", err)
		}
		if err := yaml.Unmarshal(pyBytes, &pyYAML); err != nil {
			t.Fatalf("Python Mihomo YAML invalid: %v", err)
		}

		goProxies, _ := goYAML["proxies"].([]any)
		pyProxies, _ := pyYAML["proxies"].([]any)
		if len(goProxies) != len(pyProxies) {
			t.Errorf("Mihomo proxies count mismatch: Go has %d, Python has %d", len(goProxies), len(pyProxies))
		}
	})

	t.Run("SingBox_Parity", func(t *testing.T) {
		res, err := comp.Compile(ctx, bundle, domain.TargetSingBox)
		if err != nil {
			t.Fatalf("Compile SingBox failed: %v", err)
		}
		assertNoForbiddenTokens(t, "sing-box", res.Content)

		pyFile := filepath.Join(pythonGoldenDir, "singbox.json")
		pyBytes, err := os.ReadFile(pyFile)
		if err != nil {
			t.Skipf("Python golden file %s not found: %v", pyFile, err)
		}

		var goJSON, pyJSON map[string]any
		if err := json.Unmarshal(res.Content, &goJSON); err != nil {
			t.Fatalf("Go SingBox JSON invalid: %v", err)
		}
		if err := json.Unmarshal(pyBytes, &pyJSON); err != nil {
			t.Fatalf("Python SingBox JSON invalid: %v", err)
		}

		goOutbounds, _ := goJSON["outbounds"].([]any)
		pyOutbounds, _ := pyJSON["outbounds"].([]any)
		if len(goOutbounds) < len(bundle.Nodes) || len(pyOutbounds) < len(bundle.Nodes) {
			t.Errorf("SingBox outbounds count mismatch or insufficient")
		}
	})

	t.Run("Shadowrocket_Parity", func(t *testing.T) {
		res, err := comp.Compile(ctx, bundle, domain.TargetShadowrocket)
		if err != nil {
			t.Fatalf("Compile Shadowrocket failed: %v", err)
		}
		assertNoForbiddenTokens(t, "shadowrocket", res.Content)

		pyFile := filepath.Join(pythonGoldenDir, "shadowrocket.txt")
		pyBytes, err := os.ReadFile(pyFile)
		if err != nil {
			t.Skipf("Python golden file %s not found: %v", pyFile, err)
		}

		goDecoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(res.Content)))
		if err != nil {
			t.Fatalf("Decode Go shadowrocket failed: %v", err)
		}
		pyDecoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(pyBytes)))
		if err != nil {
			t.Fatalf("Decode Python shadowrocket failed: %v", err)
		}

		goLines := strings.Split(strings.TrimSpace(string(goDecoded)), "\n")
		pyLines := strings.Split(strings.TrimSpace(string(pyDecoded)), "\n")
		if len(goLines) != len(pyLines) {
			t.Errorf("Shadowrocket lines count mismatch: Go has %d, Python has %d", len(goLines), len(pyLines))
		}
	})

	t.Run("Surge_Loon_QX_SyntaxAndRedaction", func(t *testing.T) {
		for _, target := range []domain.ExportTarget{domain.TargetSurge, domain.TargetLoon, domain.TargetQuantumultX} {
			res, err := comp.Compile(ctx, bundle, target)
			if err != nil {
				t.Fatalf("Compile %s failed: %v", target, err)
			}
			assertNoForbiddenTokens(t, string(target), res.Content)
			body := string(res.Content)
			if len(body) < 100 {
				t.Errorf("%s content too short (%d bytes)", target, len(body))
			}
		}
	})
}

func TestGoldenFixtures_ColdBackupDatabaseRegression(t *testing.T) {
	candidates := []string{
		filepath.Join("..", "..", "..", "backups", "clash_sub_parser_pre_go_rewrite_20260911.db"),
		filepath.Join("..", "..", "..", "backups", "clash_sub_parser_pre_go_rewrite.db"),
		filepath.Join("backups", "clash_sub_parser_pre_go_rewrite_20260911.db"),
		filepath.Join("backups", "clash_sub_parser_pre_go_rewrite.db"),
	}

	var dbPath string
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			dbPath = c
			break
		}
	}
	if dbPath == "" {
		t.Skip("Cold backup database not found, skipping golden DB regression test")
	}

	initialHash, err := fileSHA256(dbPath)
	if err != nil {
		t.Fatalf("failed to calculate initial db hash: %v", err)
	}

	db, err := repository.NewSQLiteDB(repository.Options{
		Path:     dbPath,
		ReadOnly: true,
	})
	if err != nil {
		t.Fatalf("Failed to open read-only sqlite db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Verify write operations are strictly blocked by read-only mode
	_, writeErr := db.ExecContext(ctx, "INSERT INTO subscriptions (name, url) VALUES ('violation', 'http://violation')")
	if writeErr == nil {
		t.Fatalf("SECURITY VIOLATION: Write operation succeeded on read-only database connection!")
	}

	repos := db.Repositories()
	comp, err := template.NewCompiler()
	if err != nil {
		t.Fatalf("Failed to create compiler: %v", err)
	}

	genSvc := service.NewGenerateService(repos, comp)

	targets := []domain.ExportTarget{
		domain.TargetClash,
		domain.TargetMihomo,
		domain.TargetSingBox,
		domain.TargetSurge,
		domain.TargetLoon,
		domain.TargetQuantumultX,
		domain.TargetShadowrocket,
	}

	for _, target := range targets {
		t.Run(string(target), func(t *testing.T) {
			req := &domain.GenerateRequest{
				Target:           target,
				IncludeUnchecked: true,
			}
			res, err := genSvc.GenerateResult(ctx, req, nil, "")
			if err != nil {
				t.Fatalf("GenerateResult for %s failed: %v", target, err)
			}
			if len(res.Content) == 0 {
				t.Fatalf("GenerateResult for %s produced empty content", target)
			}

			assertNoForbiddenTokens(t, string(target), res.Content)

			switch target {
			case domain.TargetClash, domain.TargetMihomo:
				var parsed map[string]any
				if err := yaml.Unmarshal(res.Content, &parsed); err != nil {
					t.Fatalf("%s YAML parse error: %v", target, err)
				}
				proxies, ok := parsed["proxies"].([]any)
				if !ok || len(proxies) == 0 {
					t.Errorf("%s missing proxies array", target)
				}
			case domain.TargetSingBox:
				var parsed map[string]any
				if err := json.Unmarshal(res.Content, &parsed); err != nil {
					t.Fatalf("sing-box JSON parse error: %v", err)
				}
				outbounds, ok := parsed["outbounds"].([]any)
				if !ok || len(outbounds) == 0 {
					t.Errorf("sing-box missing outbounds array")
				}
			case domain.TargetSurge, domain.TargetLoon:
				str := string(res.Content)
				if !strings.Contains(str, "[General]") || !strings.Contains(str, "[Proxy]") {
					t.Errorf("%s missing standard INI sections", target)
				}
			case domain.TargetQuantumultX:
				str := string(res.Content)
				if !strings.Contains(str, "[general]") || !strings.Contains(str, "[server_local]") {
					t.Errorf("quantumult-x missing standard sections")
				}
			case domain.TargetShadowrocket:
				decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(res.Content)))
				if err != nil {
					t.Fatalf("shadowrocket base64 decode failed: %v", err)
				}
				if len(decoded) == 0 {
					t.Errorf("shadowrocket decoded content is empty")
				}
			}

			fmt.Printf("Cold Backup DB Regression: %-14s rendered %7d bytes (%s)\n", target, len(res.Content), res.Filename)
		})
	}

	finalHash, err := fileSHA256(dbPath)
	if err != nil {
		t.Fatalf("failed to calculate final db hash: %v", err)
	}

	if initialHash != finalHash {
		t.Fatalf("DATA INTEGRITY VIOLATION: Database checksum changed during read-only verification! %s != %s", initialHash, finalHash)
	}
}
