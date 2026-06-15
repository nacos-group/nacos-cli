package skill

import "testing"

func TestIsSyncDaemonCommand(t *testing.T) {
	tests := []struct {
		name    string
		cmdline string
		want    bool
	}{
		{
			name:    "foreground skill sync daemon",
			cmdline: "/usr/local/bin/nacos-cli skill-sync start --foreground --interval 30s",
			want:    true,
		},
		{
			name:    "parent start command is not daemon",
			cmdline: "/usr/local/bin/nacos-cli skill-sync start --interval 30s",
			want:    false,
		},
		{
			name:    "foreground flag with value",
			cmdline: "/usr/local/bin/nacos-cli skill-sync start --foreground=true",
			want:    true,
		},
		{
			name:    "unrelated command",
			cmdline: "/bin/sleep 100",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSyncDaemonCommand(tt.cmdline); got != tt.want {
				t.Fatalf("isSyncDaemonCommand() = %v, want %v", got, tt.want)
			}
		})
	}
}
