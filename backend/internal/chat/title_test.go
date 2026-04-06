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
	if got := FallbackTitleFromUserMessage("Hi."); got != "Hi" {
		t.Errorf("got %q", got)
	}
	long := stringsOfRunes(100) + "."
	if got := FallbackTitleFromUserMessage(long); len([]rune(got)) != maxTitleRunes {
		t.Errorf("len=%d got %q", len([]rune(got)), got)
	}
}
