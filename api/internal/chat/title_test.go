package chat

import "testing"

func TestNormalizeChatTitle(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw      string
		fallback string
		want     string
	}{
		{`  "Hello world"  `, "x", "Hello world"},
		{"line1\nline2", "x", "line1 line2"},
		{"", "short msg", "short msg"},
		{stringsOfRunes(80), "x", stringsOfRunes(69) + "..."},
		{"What are your thoughts on cabin fever?", "x", "What are your thoughts on cabin fever?"},
	}
	for _, tc := range cases {
		got := NormalizeChatTitle(tc.raw, tc.fallback)
		if got != tc.want {
			t.Errorf("NormalizeChatTitle(%q, _) = %q want %q", tc.raw, got, tc.want)
		}
	}
}

func stringsOfRunes(n int) string {
	r := make([]rune, n)
	for i := range r {
		r[i] = 'a'
	}
	return string(r)
}

func TestFallbackTitleFromUserMessage(t *testing.T) {
	t.Parallel()
	if got := FallbackTitleFromUserMessage("Hi."); got != "Hi." {
		t.Errorf("got %q", got)
	}
	if got := FallbackTitleFromUserMessage("What are your thoughts on cabin fever?"); got != "What are your thoughts on cabin fever?" {
		t.Errorf("got %q", got)
	}
	nested := `Who says, "I am who I am?"?`
	if got := FallbackTitleFromUserMessage(nested); got != nested {
		t.Errorf("nested quote: got %q want %q", got, nested)
	}
	if got := FallbackTitleFromUserMessage(`Who says, 'I am who I am?'?`); got != `Who says, 'I am who I am?'?` {
		t.Errorf("single-quoted nested: got %q", got)
	}
	if got := FallbackTitleFromUserMessage("Why don't you ask?"); got != "Why don't you ask?" {
		t.Errorf("apostrophe: got %q", got)
	}
	long := stringsOfRunes(100) + "."
	if got := FallbackTitleFromUserMessage(long); len([]rune(got)) != maxTitleRunes {
		t.Errorf("len=%d got %q", len([]rune(got)), got)
	}
}
