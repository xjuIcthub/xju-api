package service

import (
	"fmt"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/relayconvert"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// xju-api:new — pool routing channel (#4 Phase C). A dynamically-created pool is
// only reachable for relay once a new-api channel routes its group to the pool's
// cliproxy instance. This mirrors scripts/create-k12-channel.sh in Go: clone an
// existing pool channel's model set, create an Advanced Custom channel keyed by
// the pool's internal relay key, and register the group in the ratio/usable-group
// options so cards in that group bill and route correctly. Advanced Custom keeps
// Anthropic Messages requests native while the same channel continues to expose
// CLIProxyAPI's OpenAI-compatible endpoints.

const poolChannelType = constant.ChannelTypeAdvancedCustom

const poolClaudeHeaderPassthroughRule = `re:(?i)^(anthropic-|x-claude-|x-stainless-|user-agent$)`

var poolAdvancedCustomPaths = []string{
	"/v1/messages",
	"/v1/messages/count_tokens",
	"/v1/chat/completions",
	"/v1/completions",
	"/v1/responses",
	"/v1/responses/compact",
}

var poolGroupOptionsMu sync.Mutex

func GetDefaultPoolPricingMultiplier() float64 {
	return ratio_setting.GetGroupRatio("default")
}

func SetDefaultPoolPricingMultiplier(multiplier float64) error {
	poolGroupOptionsMu.Lock()
	defer poolGroupOptionsMu.Unlock()

	groupRatios := ratio_setting.GetGroupRatioCopy()
	if groupRatios == nil {
		groupRatios = map[string]float64{}
	}
	groupRatios["default"] = multiplier
	encoded, err := common.Marshal(groupRatios)
	if err != nil {
		return err
	}
	return model.UpdateOption("GroupRatio", string(encoded))
}

func poolChannelName(poolID string) string { return "cliproxy-pool-" + poolID }

// findChannelByName returns a channel id by exact name, or 0 if none exists.
func findChannelByName(name string) int {
	var ch model.Channel
	if err := model.DB.Where("name = ?", name).Select("id").First(&ch).Error; err != nil {
		return 0
	}
	return ch.Id
}

// poolTemplateModels returns the model set to seed a new pool channel with. Every
// cliproxy pool shares the same model set, so it clones from any existing pool
// channel. It resolves the channel by a registered pool's channel id first — that
// survives a channel rename (which moves the name away from the cliproxy-pool
// prefix) — and falls back to the name prefix for any channel not yet in the
// registry. This replaces the old hard dependency on channel id 1.
func poolTemplateModels() (string, error) {
	// Primary: a registered pool's channel, resolved by id (rename-proof).
	for _, p := range common.ListConfiguredPools() {
		entry, ok := common.GetPoolEntry(p.ID)
		if !ok || entry.ChannelID == 0 {
			continue
		}
		if ch, err := model.GetChannelById(entry.ChannelID, false); err == nil && ch != nil && strings.TrimSpace(ch.Models) != "" {
			return ch.Models, nil
		}
	}
	// Fallback: any channel still using the cliproxy-pool name prefix.
	var ch model.Channel
	if err := model.DB.
		Where("type IN ? AND name LIKE ? AND models <> ''", []int{constant.ChannelTypeOpenAI, poolChannelType}, "cliproxy-pool%").
		Order("id").
		Select("models").
		First(&ch).Error; err != nil {
		return "", fmt.Errorf("no existing cliproxy pool channel to clone models from: %w", err)
	}
	return ch.Models, nil
}

// createPoolChannel creates the routing channel for a pool (idempotent by name)
// and registers its immutable group key. Private group keys are intentionally
// omitted from global UserUsableGroups and are exposed only to their owner.
func createPoolChannel(poolID, internalKey, baseURL, label, groupKey string, globallyVisible bool) (int, error) {
	name := poolChannelName(poolID)
	if existing := findChannelByName(name); existing != 0 {
		channel, err := model.GetChannelById(existing, false)
		if err != nil {
			return 0, err
		}
		if _, err = ensurePoolChannelCompatibility(channel); err != nil {
			return 0, err
		}
		return existing, nil
	}
	models, err := poolTemplateModels()
	if err != nil {
		return 0, err
	}
	groupKey = strings.TrimSpace(groupKey)
	if groupKey == "" {
		groupKey = poolID
	}
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	ch := &model.Channel{
		Type:    poolChannelType,
		Name:    name,
		Key:     internalKey,
		BaseURL: &base,
		Models:  models,
		Group:   groupKey,
		Status:  1, // enabled
	}
	applyPoolChannelCompatibility(ch)
	if err := ch.ValidateSettings(); err != nil {
		return 0, err
	}
	if err := ch.Insert(); err != nil {
		return 0, err
	}
	// Group options are non-critical: log but don't fail the channel on error.
	if err := addPoolGroupOptions(groupKey, label, globallyVisible); err != nil {
		common.SysError("pool channel: group options update failed for " + groupKey + ": " + err.Error())
	}
	return ch.Id, nil
}

func poolAdvancedCustomConfig() *dto.AdvancedCustomConfig {
	routes := make([]dto.AdvancedCustomRoute, 0, len(poolAdvancedCustomPaths))
	for _, path := range poolAdvancedCustomPaths {
		routes = append(routes, dto.AdvancedCustomRoute{
			IncomingPath: path,
			UpstreamPath: path,
			Converter:    relayconvert.ConverterNone,
		})
	}
	return &dto.AdvancedCustomConfig{Routes: routes}
}

func mergePoolAdvancedCustomConfig(existing *dto.AdvancedCustomConfig) *dto.AdvancedCustomConfig {
	if existing == nil {
		return poolAdvancedCustomConfig()
	}
	routes := append([]dto.AdvancedCustomRoute(nil), existing.Routes...)
	for _, path := range poolAdvancedCustomPaths {
		found := false
		for index := range routes {
			if strings.TrimSpace(routes[index].IncomingPath) != path || len(routes[index].Models) != 0 {
				continue
			}
			routes[index].IncomingPath = path
			routes[index].UpstreamPath = path
			routes[index].Converter = relayconvert.ConverterNone
			found = true
			break
		}
		if !found {
			routes = append(routes, dto.AdvancedCustomRoute{
				IncomingPath: path,
				UpstreamPath: path,
				Converter:    relayconvert.ConverterNone,
			})
		}
	}
	return &dto.AdvancedCustomConfig{Routes: routes}
}

// applyPoolChannelCompatibility mutates only the compatibility-owned fields.
// Identity, routing, credentials, model sets and operational state stay intact.
func applyPoolChannelCompatibility(ch *model.Channel) {
	if ch == nil {
		return
	}
	ch.Type = poolChannelType

	settings := ch.GetSetting()
	settings.PassThroughBodyEnabled = true
	ch.SetSetting(settings)

	otherSettings := ch.GetOtherSettings()
	otherSettings.AdvancedCustom = mergePoolAdvancedCustomConfig(otherSettings.AdvancedCustom)
	ch.SetOtherSettings(otherSettings)

	headerOverride := ch.GetHeaderOverride()
	headerOverride[poolClaudeHeaderPassthroughRule] = ""
	encoded, err := common.Marshal(headerOverride)
	if err == nil {
		value := string(encoded)
		ch.HeaderOverride = &value
	}
}

// deletePoolChannel removes a pool's routing channel + its group registration.
// Best-effort: it logs failures rather than blocking pool deletion.
func deletePoolChannel(poolID, groupKey string, channelID int) {
	groupKey = strings.TrimSpace(groupKey)
	if channelID == 0 {
		channelID = findChannelByName(poolChannelName(poolID))
	}
	if channelID != 0 {
		ch := model.Channel{Id: channelID}
		if err := ch.Delete(); err != nil {
			common.SysError("pool channel delete failed for " + poolID + ": " + err.Error())
		}
	}
	if groupKey == "" {
		groupKey = poolID
	}
	removePoolGroupOptions(groupKey)
}

// addPoolGroupOptions always registers billing ratio. Only shared/admin pools
// are added to global UserUsableGroups; private groups remain owner-scoped.
func addPoolGroupOptions(groupKey, label string, globallyVisible bool) error {
	poolGroupOptionsMu.Lock()
	defer poolGroupOptionsMu.Unlock()

	gr := ratio_setting.GetGroupRatioCopy()
	if gr == nil {
		gr = map[string]float64{}
	}
	if _, ok := gr[groupKey]; !ok {
		gr[groupKey] = ratio_setting.GetGroupRatio("default")
		b, err := common.Marshal(gr)
		if err != nil {
			return err
		}
		if err := model.UpdateOption("GroupRatio", string(b)); err != nil {
			return err
		}
	}
	if !globallyVisible {
		return nil
	}
	uug := setting.GetUserUsableGroupsCopy()
	if uug == nil {
		uug = map[string]string{}
	}
	if _, ok := uug[groupKey]; !ok {
		if strings.TrimSpace(label) == "" {
			label = groupKey
		}
		uug[groupKey] = label
		b, err := common.Marshal(uug)
		if err != nil {
			return err
		}
		if err := model.UpdateOption("UserUsableGroups", string(b)); err != nil {
			return err
		}
	}
	return nil
}

func removePoolGroupOptions(groupKey string) {
	poolGroupOptionsMu.Lock()
	defer poolGroupOptionsMu.Unlock()

	gr := ratio_setting.GetGroupRatioCopy()
	if _, ok := gr[groupKey]; ok {
		delete(gr, groupKey)
		if b, err := common.Marshal(gr); err == nil {
			_ = model.UpdateOption("GroupRatio", string(b))
		}
	}
	uug := setting.GetUserUsableGroupsCopy()
	if _, ok := uug[groupKey]; ok {
		delete(uug, groupKey)
		if b, err := common.Marshal(uug); err == nil {
			_ = model.UpdateOption("UserUsableGroups", string(b))
		}
	}
}

// renamePoolGroup updates a pool channel's display name. New pools carry an
// immutableGroupKey and never change routing when renamed. Legacy entries have
// no key, so they retain the historical label-as-group migration behavior.
func renamePoolGroup(poolID string, channelID int, newLabel, immutableGroupKey string, globallyVisible bool) error {
	ch := poolChannel(poolID, channelID)
	if ch == nil {
		return fmt.Errorf("routing channel not found for pool %s", poolID)
	}
	oldGroup := ch.Group
	immutableGroupKey = strings.TrimSpace(immutableGroupKey)
	if immutableGroupKey != "" {
		if oldGroup != immutableGroupKey && groupInUse(immutableGroupKey, oldGroup) {
			return fmt.Errorf("routing group %q is already used by another channel", immutableGroupKey)
		}
		ch.Name = newLabel
		ch.Group = immutableGroupKey
		if err := ch.Update(); err != nil {
			return err
		}
		if oldGroup != immutableGroupKey {
			if err := model.DB.Model(&model.Token{}).
				Where(&model.Token{Group: oldGroup}).
				Update("group", immutableGroupKey).Error; err != nil {
				common.SysError("pool routing-key migration " + oldGroup + "->" + immutableGroupKey + " failed: " + err.Error())
			}
			removePoolGroupOptions(oldGroup)
		}
		if err := addPoolGroupOptions(immutableGroupKey, newLabel, globallyVisible); err != nil {
			return err
		}
		if !globallyVisible {
			removePoolUserUsableGroup(immutableGroupKey)
		}
		return nil
	}

	newGroup := newLabel

	if oldGroup != newGroup && groupInUse(newGroup, oldGroup) {
		return fmt.Errorf("the name %q is already used by another group; pick a different name or remove that group first", newGroup)
	}

	// Channel.Update() rebuilds this channel's abilities for the (possibly new)
	// group, so relay routing follows the rename.
	ch.Name = newLabel
	ch.Group = newGroup
	if err := ch.Update(); err != nil {
		return err
	}

	if oldGroup == newGroup {
		setPoolGroupLabel(newGroup, newLabel) // only the display label changed
		return nil
	}

	// Migrate already-issued cards so their routing survives the group rename.
	if err := model.DB.Model(&model.Token{}).
		Where(&model.Token{Group: oldGroup}).
		Update("group", newGroup).Error; err != nil {
		common.SysError("pool rename: token group migration " + oldGroup + "->" + newGroup + " failed: " + err.Error())
	}
	migratePoolGroupOptions(oldGroup, newGroup, newLabel)
	return nil
}

func removePoolUserUsableGroup(groupKey string) {
	uug := setting.GetUserUsableGroupsCopy()
	if _, ok := uug[groupKey]; !ok {
		return
	}
	delete(uug, groupKey)
	if b, err := common.Marshal(uug); err == nil {
		_ = model.UpdateOption("UserUsableGroups", string(b))
	}
}

// poolChannel resolves a pool's routing channel by id, falling back to its
// cliproxy-pool-<id> name.
func poolChannel(poolID string, channelID int) *model.Channel {
	if channelID != 0 {
		if ch, err := model.GetChannelById(channelID, false); err == nil && ch != nil {
			return ch
		}
	}
	if id := findChannelByName(poolChannelName(poolID)); id != 0 {
		if ch, err := model.GetChannelById(id, false); err == nil {
			return ch
		}
	}
	return nil
}

// groupInUse reports whether `group` is already registered — as a channel group,
// a GroupRatio key, or a UserUsableGroups key — ignoring the pool's own current
// group. Used to refuse a rename that would collide two groups.
func groupInUse(group, exceptGroup string) bool {
	if strings.TrimSpace(group) == "" || group == exceptGroup {
		return false
	}
	if _, ok := setting.GetUserUsableGroupsCopy()[group]; ok {
		return true
	}
	if _, ok := ratio_setting.GetGroupRatioCopy()[group]; ok {
		return true
	}
	var ch model.Channel
	if err := model.DB.Where(&model.Channel{Group: group}).Select("id").First(&ch).Error; err == nil && ch.Id != 0 {
		return true
	}
	return false
}

// migratePoolGroupOptions moves the GroupRatio value and UserUsableGroups entry
// from oldGroup to newGroup (labelled newLabel), preserving the ratio.
func migratePoolGroupOptions(oldGroup, newGroup, newLabel string) {
	poolGroupOptionsMu.Lock()
	defer poolGroupOptionsMu.Unlock()

	gr := ratio_setting.GetGroupRatioCopy()
	if gr == nil {
		gr = map[string]float64{}
	}
	if v, ok := gr[oldGroup]; ok {
		gr[newGroup] = v
		delete(gr, oldGroup)
	} else if _, ok := gr[newGroup]; !ok {
		gr[newGroup] = ratio_setting.GetGroupRatio("default")
	}
	if b, err := common.Marshal(gr); err == nil {
		_ = model.UpdateOption("GroupRatio", string(b))
	}
	uug := setting.GetUserUsableGroupsCopy()
	if uug == nil {
		uug = map[string]string{}
	}
	delete(uug, oldGroup)
	uug[newGroup] = newLabel
	if b, err := common.Marshal(uug); err == nil {
		_ = model.UpdateOption("UserUsableGroups", string(b))
	}
}

// setPoolGroupLabel refreshes only a group's display label (its key is unchanged).
func setPoolGroupLabel(group, label string) {
	poolGroupOptionsMu.Lock()
	defer poolGroupOptionsMu.Unlock()

	uug := setting.GetUserUsableGroupsCopy()
	if uug == nil {
		uug = map[string]string{}
	}
	if uug[group] != label {
		uug[group] = label
		if b, err := common.Marshal(uug); err == nil {
			_ = model.UpdateOption("UserUsableGroups", string(b))
		}
	}
}
