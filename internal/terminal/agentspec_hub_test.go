package terminal

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/nacos-group/nacos-cli/internal/client"
)

func TestInteractiveAgentSpecListSharesHandlerAndNamespace(t *testing.T) {
	for _, input := range []string{"agentspec-list", "agentspec-list --hub", "agentspec-list --no-hub", "agentspec-list --hub --no-hub", "agentspec-list --output json"} {
		t.Run(input, func(t *testing.T) {
			c := &client.NacosClient{Namespace: "before"}
			term := NewTerminalWithProfile(c, "team")
			called := false
			term.SetAgentSpecListHandler(func(args []string) error {
				called = true
				_, want := parseCommandArgs(input)
				if !reflect.DeepEqual(args, want) || c.Namespace != "after" {
					t.Fatalf("lost args/namespace: %v %s", args, c.Namespace)
				}
				return fmt.Errorf("shared command validation detail")
			})
			output := captureTerminalStdout(t, func() {
				term.handleCommand("ns after")
				term.handleCommand(input)
			})
			if !called || !strings.Contains(output, "shared command validation detail") {
				t.Fatal("shared handler not used or error hidden")
			}
		})
	}
}

func TestInteractiveAgentSpecGetSharesSelectionHandlerAndCompletion(t *testing.T) {
	c := &client.NacosClient{Namespace: "business"}
	term := NewTerminal(c)
	called := false
	term.SetAgentSpecGetHandler(func(args []string, readSelection func(string) (string, error)) error {
		called = true
		if !reflect.DeepEqual(args, []string{"same", "749", "--version", "v2"}) || readSelection == nil || c.Namespace != "business" {
			t.Fatalf("lost shared selection: %v", args)
		}
		return fmt.Errorf("pinned version unavailable")
	})
	output := captureTerminalStdout(t, func() { term.handleCommand("agentspec-get same 749 --version v2") })
	if !called || !strings.Contains(output, "pinned version unavailable") {
		t.Fatal("download bypassed shared handler")
	}
	specs := interactiveCompletionSpecs()
	for _, name := range []string{"agentspec-list", "agentspec-get"} {
		flags := strings.Join(specs[name].Flags, " ")
		if !strings.Contains(flags, "--hub") || !strings.Contains(flags, "--no-hub") {
			t.Fatalf("missing source flags for %s", name)
		}
	}
	if !strings.Contains(strings.Join(specs["agentspec-get"].Flags, " "), "--non-interactive") {
		t.Fatal("missing non-interactive completion")
	}
}
