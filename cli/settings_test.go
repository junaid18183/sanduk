package cli

import "testing"

func TestResolveSettingPrecedence(t *testing.T) {
	const envName = "SANDUK_SETTING_TEST"

	cases := []struct {
		name     string
		flag     string
		env      string
		cfg      string
		def      string
		want     string
		wantFrom settingSource
	}{
		{"flag beats env, config and default", "f", "e", "c", "d", "f", sourceFlag},
		{"env beats config and default", "", "e", "c", "d", "e", sourceEnv},
		{"config beats default", "", "", "c", "d", "c", sourceConfig},
		{"default is the last resort", "", "", "", "d", "d", sourceDefault},
		{"nothing set at all", "", "", "", "", "", sourceDefault},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(envName, c.env)

			got, from := resolveSetting(c.flag, envName, c.cfg, c.def)
			if got != c.want || from != c.wantFrom {
				t.Fatalf("resolveSetting = (%q, %q), want (%q, %q)", got, from, c.want, c.wantFrom)
			}
		})
	}
}

func TestOrDefault(t *testing.T) {
	if got := orDefault("", "fallback"); got != "fallback" {
		t.Fatalf("orDefault(\"\") = %q, want fallback", got)
	}
	if got := orDefault("set", "fallback"); got != "set" {
		t.Fatalf("orDefault(set) = %q, want set", got)
	}
}
