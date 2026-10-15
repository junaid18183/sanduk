package cli

import (
	"testing"
)

func TestSplitExecArgs(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		dash    int
		wantSel []string
		wantCmd []string
		wantErr bool
	}{
		{
			name:    "single selector without a dash",
			args:    []string{"warden", "pnpm", "dev"},
			dash:    -1,
			wantSel: []string{"warden"},
			wantCmd: []string{"pnpm", "dev"},
		},
		{
			name:    "several selectors with a dash",
			args:    []string{"base", "warden", "pnpm", "dev"},
			dash:    2,
			wantSel: []string{"base", "warden"},
			wantCmd: []string{"pnpm", "dev"},
		},
		{
			name:    "project entry only",
			args:    []string{"pnpm", "dev"},
			dash:    0,
			wantSel: nil,
			wantCmd: []string{"pnpm", "dev"},
		},
		{
			name:    "nothing after the dash",
			args:    []string{"warden"},
			dash:    1,
			wantErr: true,
		},
		{
			name:    "no command at all",
			args:    []string{"warden"},
			dash:    -1,
			wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sels, cmd, err := splitExecArgs(c.args, c.dash)
			if c.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("splitExecArgs: %v", err)
			}
			if len(sels) != len(c.wantSel) {
				t.Fatalf("selectors = %v, want %v", sels, c.wantSel)
			}
			for i := range c.wantSel {
				if sels[i] != c.wantSel[i] {
					t.Fatalf("selectors = %v, want %v", sels, c.wantSel)
				}
			}
			if len(cmd) != len(c.wantCmd) {
				t.Fatalf("command = %v, want %v", cmd, c.wantCmd)
			}
			for i := range c.wantCmd {
				if cmd[i] != c.wantCmd[i] {
					t.Fatalf("command = %v, want %v", cmd, c.wantCmd)
				}
			}
		})
	}
}
