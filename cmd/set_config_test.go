package cmd

import "testing"

func TestSetConfigCommandHasTypeFlag(t *testing.T) {
	flag := setConfigCmd.Flags().Lookup("type")
	if flag == nil {
		t.Fatal("config-set should expose --type")
	}
	if flag.Shorthand != "t" {
		t.Fatalf("--type shorthand = %q, want t", flag.Shorthand)
	}
}
