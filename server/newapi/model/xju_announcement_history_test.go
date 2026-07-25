package model

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupAnnouncementHistoryTest(t *testing.T, notice, publishedAt, history string) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Exec("DELETE FROM options").Error)
	require.NoError(t, LOG_DB.Exec("DELETE FROM logs").Error)

	common.OptionMapRWMutex.Lock()
	previousMap := common.OptionMap
	common.OptionMap = map[string]string{
		"Notice":                     notice,
		noticePublishedAtOptionKey:   publishedAt,
		announcementHistoryOptionKey: history,
		"Announcements":              "",
	}
	common.OptionMapRWMutex.Unlock()

	previousAnnouncements := console_setting.GetConsoleSetting().Announcements
	console_setting.GetConsoleSetting().Announcements = history

	t.Cleanup(func() {
		require.NoError(t, DB.Exec("DELETE FROM options").Error)
		require.NoError(t, LOG_DB.Exec("DELETE FROM logs").Error)
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousMap
		common.OptionMapRWMutex.Unlock()
		console_setting.GetConsoleSetting().Announcements = previousAnnouncements
	})
}

func currentAnnouncementHistory(t *testing.T) []map[string]any {
	t.Helper()
	common.OptionMapRWMutex.RLock()
	raw := common.OptionMap[announcementHistoryOptionKey]
	common.OptionMapRWMutex.RUnlock()
	history, err := parseAnnouncementHistory(raw)
	require.NoError(t, err)
	return history
}

func TestUpdateNoticeWithHistoryPreservesEveryPublishedVersion(t *testing.T) {
	oldTime := time.Date(2026, time.July, 24, 5, 2, 6, 0, time.UTC)
	newTime := time.Date(2026, time.July, 25, 15, 30, 0, 123000000, time.UTC)
	setupAnnouncementHistoryTest(t, "Old notice", formatAnnouncementTime(oldTime), "[]")

	require.NoError(t, UpdateNoticeWithHistory("New notice", newTime))
	history := currentAnnouncementHistory(t)
	require.Len(t, history, 2)
	assert.Equal(t, "Old notice", history[0]["content"])
	assert.Equal(t, formatAnnouncementTime(oldTime), history[0]["publishDate"])
	assert.Equal(t, "New notice", history[1]["content"])
	assert.Equal(t, formatAnnouncementTime(newTime), history[1]["publishDate"])

	common.OptionMapRWMutex.RLock()
	assert.Equal(t, "New notice", common.OptionMap["Notice"])
	assert.Equal(t, formatAnnouncementTime(newTime), common.OptionMap[noticePublishedAtOptionKey])
	common.OptionMapRWMutex.RUnlock()

	// Saving the same current notice again is idempotent.
	require.NoError(t, UpdateNoticeWithHistory("New notice", newTime.Add(time.Hour)))
	assert.Len(t, currentAnnouncementHistory(t), 2)

	// Clearing the current banner keeps the historical timeline intact.
	require.NoError(t, UpdateNoticeWithHistory("", newTime.Add(2*time.Hour)))
	assert.Len(t, currentAnnouncementHistory(t), 2)
	common.OptionMapRWMutex.RLock()
	assert.Empty(t, common.OptionMap["Notice"])
	assert.Empty(t, common.OptionMap[noticePublishedAtOptionKey])
	common.OptionMapRWMutex.RUnlock()
}

func TestEnsureNoticeHistoryUsesLatestNoticeAuditTimeAndIsIdempotent(t *testing.T) {
	auditTime := time.Date(2026, time.July, 24, 5, 2, 6, 0, time.UTC)
	setupAnnouncementHistoryTest(t, "Legacy notice", "", "[]")
	require.NoError(t, LOG_DB.Create(&Log{
		CreatedAt: auditTime.Unix(),
		Type:      LogTypeManage,
		Other:     `{"op":{"action":"option.update","params":{"key":"Notice"}}}`,
	}).Error)

	require.NoError(t, EnsureNoticeHistory())
	history := currentAnnouncementHistory(t)
	require.Len(t, history, 1)
	assert.Equal(t, "Legacy notice", history[0]["content"])
	assert.Equal(t, formatAnnouncementTime(auditTime), history[0]["publishDate"])

	require.NoError(t, EnsureNoticeHistory())
	assert.Len(t, currentAnnouncementHistory(t), 1)
}

func TestUpdateAnnouncementHistoryPreservesPublishedNoticeEntries(t *testing.T) {
	publishedAt := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)
	currentJSON, err := common.Marshal([]map[string]any{
		{
			"id": 1, "content": "Published notice", "publishDate": formatAnnouncementTime(publishedAt),
			"type": "default", "source": "notice",
		},
		{
			"id": 2, "content": "Manual entry", "publishDate": formatAnnouncementTime(publishedAt.Add(-time.Hour)),
			"type": "success",
		},
	})
	require.NoError(t, err)
	setupAnnouncementHistoryTest(t, "Published notice", formatAnnouncementTime(publishedAt), string(currentJSON))

	// Simulate an administrator saving an older browser snapshot that omitted
	// the immutable Notice entry while editing a manual timeline item.
	submittedJSON, err := common.Marshal([]map[string]any{
		{
			"id": 2, "content": "Edited manual entry", "publishDate": formatAnnouncementTime(publishedAt.Add(-time.Hour)),
			"type": "success",
		},
		{
			"id": 1, "content": "Stale editor new entry", "publishDate": formatAnnouncementTime(publishedAt.Add(time.Hour)),
			"type": "warning",
		},
		{
			"content": "Legacy entry without id", "publishDate": formatAnnouncementTime(publishedAt.Add(-2 * time.Hour)),
			"type": "default",
		},
	})
	require.NoError(t, err)
	require.NoError(t, UpdateAnnouncementHistory(string(submittedJSON)))

	history := currentAnnouncementHistory(t)
	require.Len(t, history, 4)
	contents := map[string]bool{}
	ids := map[int64]bool{}
	for _, item := range history {
		contents[item["content"].(string)] = true
		id, ok := announcementNumericID(item)
		require.True(t, ok)
		assert.False(t, ids[id], "announcement id %d must be unique", id)
		ids[id] = true
	}
	assert.True(t, contents["Published notice"])
	assert.True(t, contents["Edited manual entry"])
	assert.True(t, contents["Stale editor new entry"])
	assert.True(t, contents["Legacy entry without id"])
}

func TestUpdateNoticeWithHistoryRejectsCorruptTimelineWithoutOverwrite(t *testing.T) {
	publishedAt := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)
	setupAnnouncementHistoryTest(t, "Current notice", formatAnnouncementTime(publishedAt), "{")

	err := UpdateNoticeWithHistory("Replacement", publishedAt.Add(time.Hour))
	require.Error(t, err)
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, "Current notice", common.OptionMap["Notice"])
	assert.Equal(t, "{", common.OptionMap[announcementHistoryOptionKey])
	common.OptionMapRWMutex.RUnlock()
}

func TestConcurrentNoticePublishesKeepEveryEntryAndUniqueID(t *testing.T) {
	setupAnnouncementHistoryTest(t, "", "", "[]")
	const publishCount = 8

	var wg sync.WaitGroup
	errors := make(chan error, publishCount)
	for i := 0; i < publishCount; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			errors <- UpdateNoticeWithHistory(
				fmt.Sprintf("Notice %d", index),
				time.Date(2026, time.July, 25, 12, 0, index, 0, time.UTC),
			)
		}(i)
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}

	history := currentAnnouncementHistory(t)
	require.Len(t, history, publishCount)
	ids := make(map[int64]bool, publishCount)
	contents := make(map[string]bool, publishCount)
	for _, item := range history {
		ids[int64(item["id"].(float64))] = true
		contents[item["content"].(string)] = true
	}
	assert.Len(t, ids, publishCount)
	assert.Len(t, contents, publishCount)
}

func TestNoticePublishAtTimelineCapacityFailsWithoutDroppingHistory(t *testing.T) {
	baseTime := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)
	history := make([]map[string]any, 0, 100)
	for index := 0; index < 100; index++ {
		history = append(history, map[string]any{
			"id":          index + 1,
			"content":     fmt.Sprintf("History %d", index),
			"publishDate": formatAnnouncementTime(baseTime.Add(time.Duration(index) * time.Minute)),
			"type":        "default",
		})
	}
	historyJSON, err := common.Marshal(history)
	require.NoError(t, err)
	setupAnnouncementHistoryTest(t, "", "", string(historyJSON))

	err = UpdateNoticeWithHistory("Would exceed capacity", baseTime.Add(101*time.Minute))
	require.Error(t, err)
	common.OptionMapRWMutex.RLock()
	assert.Empty(t, common.OptionMap["Notice"])
	assert.Equal(t, string(historyJSON), common.OptionMap[announcementHistoryOptionKey])
	common.OptionMapRWMutex.RUnlock()
}
