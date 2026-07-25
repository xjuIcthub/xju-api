package model

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"gorm.io/gorm"
)

const (
	announcementHistoryOptionKey = "console_setting.announcements"
	noticePublishedAtOptionKey   = "NoticePublishedAt"
)

var noticeHistoryMutex sync.Mutex

// UpdateNoticeWithHistory keeps the legacy Notice option for existing clients
// while appending every published version to the timeline in the same database
// transaction. The server owns the publish time so browsers cannot forge it.
func UpdateNoticeWithHistory(notice string, publishedAt time.Time) error {
	noticeHistoryMutex.Lock()
	defer noticeHistoryMutex.Unlock()

	return updateNoticeWithHistoryLocked(notice, publishedAt)
}

// UpdateAnnouncementHistory serializes manual timeline edits with Notice
// publishing, preventing the two administrator editors from overwriting each
// other when they save at the same time.
func UpdateAnnouncementHistory(raw string) error {
	noticeHistoryMutex.Lock()
	defer noticeHistoryMutex.Unlock()

	submitted, err := parseAnnouncementHistory(raw)
	if err != nil {
		return err
	}
	_, _, currentRaw := noticeHistorySnapshot()
	current, err := parseAnnouncementHistory(currentRaw)
	if err != nil {
		return err
	}

	merged := preservePublishedNoticeEntries(submitted, current)
	mergedJSON, err := common.Marshal(merged)
	if err != nil {
		return err
	}
	if err := console_setting.ValidateConsoleSettings(string(mergedJSON), "Announcements"); err != nil {
		return err
	}
	return persistAnnouncementOptions(map[string]string{
		announcementHistoryOptionKey: string(mergedJSON),
	})
}

// EnsureNoticeHistory performs the one-time, idempotent migration for sites
// that already have a Notice but an empty timeline. When possible it recovers
// the original publish time from the management audit log.
func EnsureNoticeHistory() error {
	noticeHistoryMutex.Lock()
	defer noticeHistoryMutex.Unlock()

	common.OptionMapRWMutex.RLock()
	notice := common.OptionMap["Notice"]
	common.OptionMapRWMutex.RUnlock()
	if strings.TrimSpace(notice) == "" {
		return nil
	}

	return updateNoticeWithHistoryLocked(notice, time.Now().UTC())
}

func updateNoticeWithHistoryLocked(notice string, publishedAt time.Time) error {
	currentNotice, currentPublishedAt, historyRaw := noticeHistorySnapshot()
	history, err := parseAnnouncementHistory(historyRaw)
	if err != nil {
		return err
	}

	historyChanged := false
	currentTime := parseAnnouncementTime(currentPublishedAt)
	if strings.TrimSpace(currentNotice) != "" {
		if existingTime, exists := findAnnouncementTime(history, currentNotice); exists {
			if currentTime.IsZero() {
				currentTime = existingTime
			}
			if markMatchingNoticeAsPublished(history, currentNotice) {
				historyChanged = true
			}
		} else {
			if currentTime.IsZero() {
				currentTime = latestNoticeAuditTime()
			}
			if currentTime.IsZero() {
				currentTime = publishedAt
			}
			history = appendAnnouncement(history, currentNotice, currentTime)
			historyChanged = true
		}
	}

	nextNotice := strings.TrimSpace(notice)
	currentNormalized := strings.TrimSpace(currentNotice)
	nextPublishedAt := currentTime
	if nextNotice == "" {
		nextPublishedAt = time.Time{}
	} else if nextNotice != currentNormalized {
		nextPublishedAt = publishedAt
		history = appendAnnouncement(history, notice, nextPublishedAt)
		historyChanged = true
	} else if nextPublishedAt.IsZero() {
		nextPublishedAt = publishedAt
		if _, exists := findAnnouncementTime(history, notice); !exists {
			history = appendAnnouncement(history, notice, nextPublishedAt)
			historyChanged = true
		}
	}

	values := map[string]string{
		"Notice":                   notice,
		noticePublishedAtOptionKey: formatAnnouncementTime(nextPublishedAt),
	}
	if historyChanged {
		historyJSON, err := common.Marshal(history)
		if err != nil {
			return err
		}
		if err := console_setting.ValidateConsoleSettings(string(historyJSON), "Announcements"); err != nil {
			return err
		}
		values[announcementHistoryOptionKey] = string(historyJSON)
	}

	if !historyChanged && notice == currentNotice && formatAnnouncementTime(nextPublishedAt) == currentPublishedAt {
		return nil
	}
	return persistAnnouncementOptions(values)
}

func persistAnnouncementOptions(values map[string]string) error {
	if len(values) == 0 {
		return nil
	}
	if err := DB.Transaction(func(tx *gorm.DB) error {
		for key, value := range values {
			option := Option{Key: key}
			if err := tx.FirstOrCreate(&option, Option{Key: key}).Error; err != nil {
				return err
			}
			option.Value = value
			if err := tx.Save(&option).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}

	// These three keys are read together by the public announcement surfaces.
	// Apply their in-memory state under one lock after the database transaction
	// commits, avoiding a visible new-Notice/old-Timeline split.
	common.OptionMapRWMutex.Lock()
	for key, value := range values {
		common.OptionMap[key] = value
	}
	if history, ok := values[announcementHistoryOptionKey]; ok {
		console_setting.GetConsoleSetting().Announcements = history
	}
	common.OptionMapRWMutex.Unlock()
	return nil
}

func noticeHistorySnapshot() (notice, publishedAt, history string) {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()

	notice = common.OptionMap["Notice"]
	publishedAt = common.OptionMap[noticePublishedAtOptionKey]
	history = common.OptionMap[announcementHistoryOptionKey]
	if strings.TrimSpace(history) == "" {
		history = common.OptionMap["Announcements"]
	}
	return
}

func parseAnnouncementHistory(raw string) ([]map[string]any, error) {
	if strings.TrimSpace(raw) == "" {
		return []map[string]any{}, nil
	}

	var history []map[string]any
	if err := common.Unmarshal([]byte(raw), &history); err != nil {
		return nil, fmt.Errorf("parse announcement history: %w", err)
	}
	if history == nil {
		history = []map[string]any{}
	}
	return history, nil
}

func preservePublishedNoticeEntries(submitted, current []map[string]any) []map[string]any {
	protected := make(map[int64]map[string]any)
	for _, item := range current {
		if id, ok := announcementNumericID(item); ok && isPublishedNoticeEntry(item) {
			protected[id] = item
		}
	}

	merged := make([]map[string]any, 0, len(submitted)+len(protected))
	seenProtected := make(map[int64]bool, len(protected))
	usedIDs := make(map[int64]bool, len(submitted)+len(protected))
	nextID := nextAnnouncementID(append(append([]map[string]any{}, current...), submitted...))
	for _, item := range submitted {
		id, hasID := announcementNumericID(item)
		if existing, protectedID := protected[id]; hasID && protectedID {
			if isPublishedNoticeEntry(item) || sameAnnouncementVersion(item, existing) {
				merged = append(merged, existing)
				seenProtected[id] = true
				usedIDs[id] = true
				continue
			}
			// A stale editor can allocate the same max+1 id as a newly
			// published Notice. Preserve both by assigning the manual item a
			// fresh server-side id instead of silently dropping it.
			hasID = false
		}

		copyItem := cloneAnnouncement(item)
		if isPublishedNoticeEntry(item) {
			// Only the server may create immutable Notice history entries.
			delete(copyItem, "source")
		}
		if !hasID || id <= 0 || usedIDs[id] {
			id = nextID
			nextID++
			copyItem["id"] = id
		}
		usedIDs[id] = true
		merged = append(merged, copyItem)
	}
	for id, item := range protected {
		if !seenProtected[id] {
			merged = append(merged, item)
		}
	}
	return merged
}

func isPublishedNoticeEntry(item map[string]any) bool {
	source, _ := item["source"].(string)
	return source == "notice"
}

func announcementNumericID(item map[string]any) (int64, bool) {
	var id int64
	switch value := item["id"].(type) {
	case float64:
		id = int64(value)
	case int:
		id = int64(value)
	case int64:
		id = value
	case string:
		var err error
		id, err = strconv.ParseInt(value, 10, 64)
		if err != nil {
			return 0, false
		}
	default:
		return 0, false
	}
	return id, id > 0
}

func sameAnnouncementVersion(left, right map[string]any) bool {
	leftContent, _ := left["content"].(string)
	rightContent, _ := right["content"].(string)
	leftDate, _ := left["publishDate"].(string)
	rightDate, _ := right["publishDate"].(string)
	return leftContent == rightContent && leftDate == rightDate
}

func cloneAnnouncement(item map[string]any) map[string]any {
	copyItem := make(map[string]any, len(item))
	for key, value := range item {
		copyItem[key] = value
	}
	return copyItem
}

func appendAnnouncement(history []map[string]any, content string, publishedAt time.Time) []map[string]any {
	return append(history, map[string]any{
		"id":          nextAnnouncementID(history),
		"content":     content,
		"publishDate": formatAnnouncementTime(publishedAt),
		"type":        "default",
		"source":      "notice",
	})
}

func nextAnnouncementID(history []map[string]any) int64 {
	var maxID int64
	for _, item := range history {
		var id int64
		switch value := item["id"].(type) {
		case float64:
			id = int64(value)
		case int:
			id = int64(value)
		case int64:
			id = value
		case string:
			id, _ = strconv.ParseInt(value, 10, 64)
		}
		if id > maxID {
			maxID = id
		}
	}
	return maxID + 1
}

func findAnnouncementTime(history []map[string]any, notice string) (time.Time, bool) {
	normalized := strings.TrimSpace(notice)
	var latest time.Time
	for _, item := range history {
		content, _ := item["content"].(string)
		if strings.TrimSpace(content) != normalized {
			continue
		}
		publishedAt, _ := item["publishDate"].(string)
		parsed := parseAnnouncementTime(publishedAt)
		if parsed.After(latest) {
			latest = parsed
		}
	}
	return latest, !latest.IsZero()
}

func markMatchingNoticeAsPublished(history []map[string]any, notice string) bool {
	normalized := strings.TrimSpace(notice)
	latestIndex := -1
	var latest time.Time
	for index, item := range history {
		content, _ := item["content"].(string)
		if strings.TrimSpace(content) != normalized {
			continue
		}
		publishedAt, _ := item["publishDate"].(string)
		parsed := parseAnnouncementTime(publishedAt)
		if latestIndex == -1 || parsed.After(latest) {
			latestIndex = index
			latest = parsed
		}
	}
	if latestIndex < 0 || isPublishedNoticeEntry(history[latestIndex]) {
		return false
	}
	history[latestIndex]["source"] = "notice"
	return true
}

func latestNoticeAuditTime() time.Time {
	if LOG_DB == nil {
		return time.Time{}
	}

	var log Log
	err := LOG_DB.
		Where("type = ? AND other LIKE ?", LogTypeManage, `%"key":"Notice"%`).
		Order("created_at DESC").
		First(&log).Error
	if err != nil || log.CreatedAt <= 0 {
		return time.Time{}
	}
	return time.Unix(log.CreatedAt, 0).UTC()
}

func parseAnnouncementTime(raw string) time.Time {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}

func formatAnnouncementTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
