package settings

import "testing"

func TestValidate(t *testing.T) {
	if err := Defaults().Validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Settings){
		func(s *Settings) { s.Reading.FontSize = 0 }, func(s *Settings) { s.Audio.Master = 2 },
		func(s *Settings) { s.Play.Reasoning = "unknown" }, func(s *Settings) { s.Play.Helpers = true },
		func(s *Settings) { s.Play.TitleOnExit = "auto" },
	} {
		s := Defaults()
		change(&s)
		if s.Validate() == nil {
			t.Errorf("非法设置被接受：%+v", s)
		}
	}
}
