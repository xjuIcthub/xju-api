package console_setting

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateAnnouncementsAllowsLongMarkdownAndHTML(t *testing.T) {
	content := strings.Repeat("这是一段 **Markdown** 与 <strong>HTML</strong> 公告内容。\n", 200)
	extra := strings.Repeat("补充说明\n", 200)
	payload, err := json.Marshal([]map[string]interface{}{
		{
			"id":          1,
			"content":     content,
			"publishDate": time.Now().UTC().Format(time.RFC3339),
			"type":        "success",
			"extra":       extra,
		},
	})
	if err != nil {
		t.Fatalf("marshal announcement: %v", err)
	}

	if err := ValidateConsoleSettings(string(payload), "Announcements"); err != nil {
		t.Fatalf("expected long announcement to be accepted, got %v", err)
	}
}

func TestGetAnnouncementsSortsNewestFirstAndUsesIDAsTieBreaker(t *testing.T) {
	previous := GetConsoleSetting().Announcements
	t.Cleanup(func() { GetConsoleSetting().Announcements = previous })

	publishedAt := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC).Format(time.RFC3339)
	payload, err := json.Marshal([]map[string]interface{}{
		{"id": 1, "content": "older id", "publishDate": publishedAt, "type": "default"},
		{"id": 2, "content": "newer id", "publishDate": publishedAt, "type": "default"},
		{"id": 3, "content": "newest time", "publishDate": time.Date(2026, time.July, 26, 12, 0, 0, 0, time.UTC).Format(time.RFC3339), "type": "default"},
	})
	require.NoError(t, err)
	GetConsoleSetting().Announcements = string(payload)

	announcements := GetAnnouncements()
	require.Len(t, announcements, 3)
	assert.Equal(t, "newest time", announcements[0]["content"])
	assert.Equal(t, "newer id", announcements[1]["content"])
	assert.Equal(t, "older id", announcements[2]["content"])
}
