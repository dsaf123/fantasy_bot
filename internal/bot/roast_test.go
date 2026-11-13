package bot

import (
	"context"
	"strings"
	"testing"
)

func TestRoastWithoutLLMExplainsInsteadOfErroring(t *testing.T) {
	b := &Bot{}
	got, err := b.Roast(context.Background(), "Alice", []string{"Bob"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "LLM_API_KEY") {
		t.Errorf("roast = %q, want an explanation mentioning LLM_API_KEY", got)
	}
}

func TestBuildRoastPrompt(t *testing.T) {
	p := buildRoastPrompt("Alice", []string{"Commish Bob", "bob"}, "Bob's Burgers (bob)", "Standings:\n1. Alice")
	for _, want := range []string{
		"Requested by: Alice\n",
		"Target team: Bob's Burgers (bob)\n",
		"Target's Discord names: Commish Bob, bob\n",
		"=== League data ===\nStandings:",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q:\n%s", want, p)
		}
	}

	p = buildRoastPrompt("Alice", nil, "Bob's Burgers", "data")
	if strings.Contains(p, "Discord names") {
		t.Errorf("no target names should omit that line:\n%s", p)
	}
	p = buildRoastPrompt("Alice", []string{"bob"}, "", "data")
	if strings.Contains(p, "Target team") {
		t.Errorf("no team should omit that line:\n%s", p)
	}
}
