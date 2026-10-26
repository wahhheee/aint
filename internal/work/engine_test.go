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

func TestCompileEngineRange(t *testing.T) {
	for _, tc := range []struct {
		name, expr string
		invalid    bool
	}{
		{name: "多开场能力", expr: ">=2.0 <3.0"},
		{name: "不支持的能力范围", expr: ">=1.0 <2.0", invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			set := NewFileSet()
			manifest := "format: 1\nid: test.engine\nversion: 1.0.0\ntitle: 能力版本\nopenings:\n  - {id: default, title: 故事开场, mode: generate}\nengine: '" + tc.expr + "'\n"
			if err := set.Add("work.yaml", []byte(manifest)); err != nil {
				t.Fatal(err)
			}
			_, report := Compile(set)
			if report.HasErrors() != tc.invalid {
				t.Fatalf("能力版本校验结果不符：%+v", report.Issues)
			}
			if tc.invalid && report.Issues[0].Code != CodeEngine {
				t.Fatal("应报告作品能力版本不兼容", report.Issues)
			}
		})
	}
}
