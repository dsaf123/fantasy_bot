package discordbot

import (
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"

	"fantasy_bot/internal/config"
)

func TestCleanQuestion(t *testing.T) {
	mentions := []*discordgo.User{
		{ID: "100", Username: "fantasybot"},
		{ID: "200", Username: "bob", GlobalName: "Bobby"},
	}
	tests := []struct {
		name, in, want string
	}{
		{"strips bot mention", "<@100> who's in first?", "who's in first?"},
		{"strips legacy nick mention", "<@!100>   who's in first?", "who's in first?"},
		{"mention mid-sentence", "hey <@100> is <@200> cooked?", "hey is @Bobby cooked?"},
		{"drops role mentions", "<@&555> who's in first?", "who's in first?"},
		{"unknown user mention left as-is", "<@100> what about <@999>?", "what about <@999>?"},
		{"only a mention", "<@100>", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanQuestion(tt.in, "100", mentions); got != tt.want {
				t.Errorf("cleanQuestion(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestMentionsUser(t *testing.T) {
	users := []*discordgo.User{{ID: "1"}, nil, {ID: "2"}}
	if !mentionsUser(users, "2") {
		t.Error("expected user 2 to be found")
	}
	if mentionsUser(users, "3") {
		t.Error("user 3 should not be found")
	}
}

func TestSplitMessage(t *testing.T) {
	if got := splitMessage("short", 2000); len(got) != 1 || got[0] != "short" {
		t.Errorf("short message split = %q", got)
	}

	long := strings.Repeat("word ", 1000) // 5000 chars
	chunks := splitMessage(long, 2000)
	if len(chunks) != 3 {
		t.Fatalf("got %d chunks, want 3", len(chunks))
	}
	for i, c := range chunks {
		if n := len([]rune(c)); n > 2000 {
			t.Errorf("chunk %d is %d chars, over the limit", i, n)
		}
		if strings.HasPrefix(c, " ") || strings.HasSuffix(c, "wor") {
			t.Errorf("chunk %d split mid-word or kept leading space: %q...", i, c[:20])
		}
	}
	if strings.Join(chunks, " ") != strings.TrimSpace(long) {
		t.Error("chunks don't reassemble into the original text")
	}

	lines := strings.Repeat("a line of text\n", 200)
	for i, c := range splitMessage(lines, 100) {
		if !strings.HasSuffix(c, "text") {
			t.Errorf("chunk %d didn't break on a line boundary: %q", i, c)
		}
	}

	unbroken := strings.Repeat("é", 4500) // multi-byte runes, no break points
	chunks = splitMessage(unbroken, 2000)
	if len(chunks) != 3 || len([]rune(chunks[0])) != 2000 {
		t.Errorf("hard-split of unbroken text = %d chunks (first %d runes)", len(chunks), len([]rune(chunks[0])))
	}
}

func TestCooldowns(t *testing.T) {
	c := newCooldowns(20 * time.Second)
	start := time.Now()

	if !c.allow("alice", start) {
		t.Fatal("first question should be allowed")
	}
	if c.allow("alice", start.Add(5*time.Second)) {
		t.Error("second question within the window should be blocked")
	}
	if !c.allow("bob", start.Add(5*time.Second)) {
		t.Error("a different user shouldn't share alice's cooldown")
	}
	if !c.allow("alice", start.Add(21*time.Second)) {
		t.Error("question after the window should be allowed")
	}
}

func TestReferencedText(t *testing.T) {
	m := &discordgo.Message{ReferencedMessage: &discordgo.Message{
		Content: "Team A is 7-1.",
		Author:  &discordgo.User{Username: "fantasybot"},
	}}
	if got := referencedText(m); got != "fantasybot: Team A is 7-1." {
		t.Errorf("referencedText = %q", got)
	}
	if got := referencedText(&discordgo.Message{}); got != "" {
		t.Errorf("non-reply referencedText = %q, want empty", got)
	}
}

func TestAskerNamePrefersServerNick(t *testing.T) {
	m := &discordgo.Message{Author: &discordgo.User{Username: "bob", GlobalName: "Bobby"}}
	if got := askerName(m); got != "Bobby" {
		t.Errorf("askerName without nick = %q, want Bobby", got)
	}
	m.Member = &discordgo.Member{Nick: "Commish Bob"}
	if got := askerName(m); got != "Commish Bob" {
		t.Errorf("askerName with nick = %q, want Commish Bob", got)
	}
}

func TestHandleSubcommandMentionsToggle(t *testing.T) {
	cfg := &config.Config{Timezone: "UTC"} // no LLM key: default is off
	mgr := newTestManager(t)

	content, err := handleSubcommand(cfg, mgr, sub("mentions", boolOpt("enabled", true)))
	if err != nil {
		t.Fatal(err)
	}
	if got := mgr.Get().MentionReplies; got == nil || !*got {
		t.Fatalf("MentionReplies = %v, want override true", got)
	}
	if !strings.Contains(content, "LLM_API_KEY") {
		t.Errorf("enabling without an LLM key should warn, got: %s", content)
	}
	if view := buildView(cfg, mgr.Get()); !strings.Contains(view, "**@mention replies:** on _(override)_") {
		t.Errorf("view doesn't show the override: %s", view)
	}

	// Turning it back off matches the default, so the override is cleared.
	if _, err := handleSubcommand(cfg, mgr, sub("mentions", boolOpt("enabled", false))); err != nil {
		t.Fatal(err)
	}
	if got := mgr.Get().MentionReplies; got != nil {
		t.Errorf("MentionReplies = %v, want nil after returning to default", *got)
	}
}
