package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/nacos-group/nacos-cli/internal/skill"
)

func appendSkillFailure(failures *[]string, skillName string, err error) {
	fmt.Fprintf(os.Stderr, "Error: %s: %v\n", skillName, err)
	*failures = append(*failures, fmt.Sprintf("%s: %v", skillName, err))
}

func saveSyncStateAfterBatch(state *skill.SyncState, failures []string) error {
	saveErr := skill.SaveSyncState(state)
	if len(failures) == 0 {
		return saveErr
	}
	if saveErr != nil {
		return fmt.Errorf("%d skill(s) failed: %s; additionally failed to save sync state: %w",
			len(failures), strings.Join(failures, "; "), saveErr)
	}
	return fmt.Errorf("%d skill(s) failed: %s", len(failures), strings.Join(failures, "; "))
}
