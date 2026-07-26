package controller

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

func GetGroups(c *gin.Context) {
	groupNames := make([]string, 0)
	for groupName := range ratio_setting.GetGroupRatioCopy() {
		groupNames = append(groupNames, groupName)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    groupNames,
	})
}

func GetUserGroups(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    getUserTokenGroups(c.GetInt("id")),
	})
}

// getUserTokenGroups is the single source of truth for API-key routing choices.
// Shared choices come from the durable pool registry instead of blindly
// exposing every historical ratio/UserUsableGroups entry. This keeps retired
// groups hidden while allowing newly provisioned Codex and Claude pools to be
// selected immediately.
func getUserTokenGroups(userID int) map[string]map[string]interface{} {
	usableGroups := make(map[string]map[string]interface{})
	userGroup := ""
	userGroup, _ = model.GetUserGroup(userID, false)
	userUsableGroups := service.GetUserUsableGroups(userGroup)
	// The primary default route remains available even when pool management is
	// temporarily unconfigured. API-key routing must not depend on the health of
	// the separate management surface.
	if desc, ok := userUsableGroups["default"]; ok && ratio_setting.ContainsGroupRatio("default") {
		usableGroups["default"] = map[string]interface{}{
			"ratio":    service.GetUserGroupRatio(userGroup, "default"),
			"desc":     desc,
			"provider": common.PoolProviderCodex,
		}
	}
	for _, pool := range common.ListSharedPools() {
		groupKey := strings.TrimSpace(pool.GroupKey)
		if groupKey == "" {
			groupKey = strings.TrimSpace(pool.ID)
		}
		if groupKey == "" || !ratio_setting.ContainsGroupRatio(groupKey) {
			continue
		}
		desc, visible := userUsableGroups[groupKey]
		if !visible {
			continue
		}
		if strings.TrimSpace(pool.Label) != "" {
			desc = pool.Label
		}
		usableGroups[groupKey] = map[string]interface{}{
			"ratio":    service.GetUserGroupRatio(userGroup, groupKey),
			"desc":     desc,
			"provider": pool.Provider,
		}
	}
	// Private groups never live in global UserUsableGroups. Add exactly the
	// authenticated user's ready pool here so group discovery remains owner-bound.
	if entry, ok := common.FindPrivatePoolByOwner(userID); ok && entry.ChannelID > 0 {
		if _, _, ready := common.ResolvePoolMgmt(entry.ID); ready && ratio_setting.ContainsGroupRatio(entry.GroupKey) {
			desc := entry.Label
			if desc == "" {
				desc = "我的私人号池"
			}
			usableGroups[entry.GroupKey] = map[string]interface{}{
				"ratio": service.GetUserGroupRatio(userGroup, entry.GroupKey),
				"desc":  desc,
			}
		}
	}
	return usableGroups
}

// resolveUserTokenGroup validates a requested API-key group and supplies the
// creation default: own private pool first, then the shared default pool.
func resolveUserTokenGroup(userID int, requested string) (string, bool) {
	groups := getUserTokenGroups(userID)
	requested = strings.TrimSpace(requested)
	if requested == "" {
		privateGroup := common.PrivatePoolGroupKey(userID)
		if _, ok := groups[privateGroup]; ok {
			return privateGroup, true
		}
		if _, ok := groups["default"]; ok {
			return "default", true
		}
		return "", false
	}
	_, ok := groups[requested]
	return requested, ok
}
