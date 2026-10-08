package domain

import (
	"strings"
	"testing"
	"time"
)

func TestCheapOriginalPCREGolden(t *testing.T) {
	// Frozen archived expression: literal 去掉 prefix and OR are intentional.
	pattern := `(?:去掉(流媒体|x(?:[0-5](?:\.[0-9]+)?)(?![\d.])|便宜|free ))|(?:Eeox)|(?:einck)`
	for _, tc := range []struct {
		name string
		want bool
	}{
		{"x5", false}, {"x5.9", false}, {"x6", false}, {"x50", false}, {"x5.9.1", false},
		{"去掉x5", true}, {"去掉x5.9", true}, {"去掉x6", false}, {"去掉x50", false}, {"去掉x5.9.1", false},
		{"去掉流媒体", true}, {"便宜", false}, {"去掉便宜", true}, {"去掉free ", true}, {"去掉free", false},
		{"Eeox 香港", true}, {"EINCK 日本", true}, {"其他中文节点", false}, {"前缀去掉X5 后缀", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := FilterCondition{Field: FilterFieldDisplayName, Op: FilterOpRegex, Value: pattern}
			if err := c.Validate(); err != nil {
				t.Fatal(err)
			}
			got, reason := MatchesCondition(c, Node{DisplayName: tc.name}, nil, nil, time.Time{})
			if got != tc.want {
				t.Fatalf("got %v, want %v: %s", got, tc.want, reason)
			}
			c.Op = FilterOpNotRegex
			inverse, _ := MatchesCondition(c, Node{DisplayName: tc.name}, nil, nil, time.Time{})
			if inverse == got {
				t.Fatal("not_regex did not preserve inverse semantic")
			}
		})
	}
}

func TestPCREBoundsAndTimeoutFailClosed(t *testing.T) {
	for _, op := range []FilterOp{FilterOpRegex, FilterOpNotRegex} {
		c := FilterCondition{Field: FilterFieldDisplayName, Op: op, Value: `^(?=a)(a+)+$`}
		if err := c.Validate(); err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		matched, reason := MatchesCondition(c, Node{DisplayName: strings.Repeat("a", 2000) + "!"}, nil, nil, time.Time{})
		if matched || !strings.Contains(reason, "match failed") {
			t.Fatalf("timeout inverted or hidden: %v %s", matched, reason)
		}
		if time.Since(start) > time.Second {
			t.Fatal("match timeout unbounded")
		}
		matched, reason = MatchesCondition(c, Node{DisplayName: strings.Repeat("a", filterRegexInputLimit+1)}, nil, nil, time.Time{})
		if matched || !strings.Contains(reason, "exceeds") {
			t.Fatalf("input bound not enforced: %v %s", matched, reason)
		}
	}
	if _, err := compileFilterRegex(strings.Repeat("a", 1025)); err == nil {
		t.Fatal("unbounded pattern")
	}
	for i := 0; i < 140; i++ {
		_, _ = compileFilterRegex(strings.Repeat("a", i+1))
	}
	if filterRegexCache.Len() > 128 {
		t.Fatal("unbounded cache")
	}
}
