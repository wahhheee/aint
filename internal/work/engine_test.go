package work

import "testing"

func TestEngineSatisfies(t *testing.T) {
	cases := []struct {
		expr, version string
		want          bool
	}{
		{">=1.0 <2.0", "1.0", true},
		{">=1.0 <2.0", "1.3", true},
		{">=1.0 <2.0", "2.0", false},
		{"^1.1", "1.0", false},
		{"^1.1", "1.4", true},
		{"1.0", "1.0.0", true},
		{">=2.0 || ^1", "1.2", true},
	}
	for _, c := range cases {
		got, err := engineSatisfies(c.expr, c.version)
		if err != nil || got != c.want {
			t.Errorf("engineSatisfies(%q, %q) = %v, %v；期望 %v", c.expr, c.version, got, err, c.want)
		}
	}
	if _, err := engineSatisfies(">=one", "1.0"); err == nil {
		t.Error("非法范围应报错")
	}
}
