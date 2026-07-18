package gateway

import "testing"

func TestParseModelID(t *testing.T) {
	base, adapter := ParseModelID("qwen3.6:27b")
	if base != "qwen3.6:27b" || adapter != "" {
		t.Fatalf("got %q %q", base, adapter)
	}
	base, adapter = ParseModelID("llama3.2#sql")
	if base != "llama3.2" || adapter != "sql" {
		t.Fatalf("got %q %q", base, adapter)
	}
}

func TestMessagesToPrompt(t *testing.T) {
	got := messagesToPrompt([]chatMessage{
		{Role: "user", Content: "Say hi"},
	})
	want := "user: Say hi\nassistant: "
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
