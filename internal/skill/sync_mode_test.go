package skill

import (
	"strings"
	"testing"
)

func TestSetModeNacosWithoutUsableProfileMentionsProfileFlag(t *testing.T) {
	withSkillTempHome(t)

	state := defaultSyncState()
	err := SetMode(state, SyncModeNacos, "")
	if err == nil {
		t.Fatal("expected missing profile error")
	}
	for _, want := range []string{"Nacos mode", "--profile <profile>", "profile set <profile>"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want to contain %q", err.Error(), want)
		}
	}
}
