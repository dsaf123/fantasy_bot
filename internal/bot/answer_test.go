package bot

import (
	"context"
	"strings"
	"testing"
)

func TestAnswerWithoutLLMExplainsInsteadOfErroring(t *testing.T) {
	b := &Bot{} // no LLM configured; must not touch Sleeper either
	got, err := b.Answer(context.Background(), "Bob", "who's in first?", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "LLM_API_KEY") {
		t.Errorf("answer = %q, want an explanation mentioning LLM_API_KEY", got)
	}
}

func TestBuildChatPrompt(t *testing.T) {
	p := buildChatPrompt("Bob", "  am I cooked?  ", "fantasybot: You're 1-7.", "Standings:\n1. Alice")
	for _, want := range []string{
		"Asked by: Bob\n",
		"They're replying to this earlier message:\nfantasybot: You're 1-7.\n",
		"Question: am I cooked?\n",
		"=== League data ===\nStandings:\n1. Alice",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q:\n%s", want, p)
		}
	}
	if strings.Index(p, "Question:") > strings.Index(p, "=== League data ===") {
		t.Error("question should come before the league data")
	}

	if p := buildChatPrompt("Bob", "hi", "", "data"); strings.Contains(p, "replying to") {
		t.Errorf("no reply context should be omitted entirely:\n%s", p)
	}

	long := strings.Repeat("x", maxQuestionLen+500)
	if p := buildChatPrompt("Bob", long, "", ""); strings.Count(p, "x") != maxQuestionLen {
		t.Errorf("question not truncated to %d chars", maxQuestionLen)
	}
}
