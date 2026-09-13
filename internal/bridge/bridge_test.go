package bridge

import (
	"strings"
	"testing"
)

func TestDecode(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    Message
		wantErr bool
	}{
		{"ready", `{"type":"ready"}`, Message{Type: KindReady}, false},
		{"unread", `{"type":"unread","unread":4}`, Message{Type: KindUnread, Unread: 4}, false},
		{
			"external",
			`{"type":"open_external","url":"https://example.com"}`,
			Message{Type: KindOpenExternal, URL: "https://example.com"},
			false,
		},
		{"unknown type", `{"type":"eval","url":"x"}`, Message{}, true},
		{"malformed", `not json`, Message{}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Decode(tc.payload)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Decode error = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestFocusChatQuotesTag(t *testing.T) {
	got := FocusChat(`"); alert(1); //`)
	if strings.Contains(got, "alert(1)") && !strings.Contains(got, `\"`) {
		t.Fatalf("tag was not escaped: %s", got)
	}
	if !strings.HasPrefix(got, "window.__zealish") {
		t.Errorf("unexpected script: %s", got)
	}
}
