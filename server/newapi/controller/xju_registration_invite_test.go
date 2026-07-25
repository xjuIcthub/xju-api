package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type registrationInviteOAuthProvider struct {
	existingUserID int
}

func (provider *registrationInviteOAuthProvider) GetName() string { return "InviteTest" }
func (provider *registrationInviteOAuthProvider) IsEnabled() bool { return true }
func (provider *registrationInviteOAuthProvider) ExchangeToken(context.Context, string, *gin.Context) (*oauth.OAuthToken, error) {
	return nil, errors.New("not used")
}
func (provider *registrationInviteOAuthProvider) GetUserInfo(context.Context, *oauth.OAuthToken) (*oauth.OAuthUser, error) {
	return nil, errors.New("not used")
}
func (provider *registrationInviteOAuthProvider) IsUserIDTaken(string) bool {
	return provider.existingUserID > 0
}
func (provider *registrationInviteOAuthProvider) FillUserByProviderID(user *model.User, _ string) error {
	return model.DB.First(user, provider.existingUserID).Error
}
func (provider *registrationInviteOAuthProvider) SetProviderUserID(user *model.User, providerUserID string) {
	user.GitHubId = providerUserID
}
func (provider *registrationInviteOAuthProvider) GetProviderPrefix() string { return "oit_" }

func withRegistrationInviteTestSettings(t *testing.T) {
	t.Helper()
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&model.InviteCode{},
		&model.InviteReward{},
		&model.Token{},
		&model.Log{},
		&model.CustomOAuthProvider{},
		&model.UserOAuthBinding{},
	))

	previousRegisterEnabled := common.RegisterEnabled
	previousPasswordRegisterEnabled := common.PasswordRegisterEnabled
	previousEmailVerificationEnabled := common.EmailVerificationEnabled
	previousInviteCodeRequired := common.InviteCodeRequired
	previousQuotaForNewUser := common.QuotaForNewUser
	previousQuotaPerUnit := common.QuotaPerUnit
	previousGenerateDefaultToken := constant.GenerateDefaultToken

	common.RegisterEnabled = true
	common.PasswordRegisterEnabled = true
	common.EmailVerificationEnabled = false
	common.InviteCodeRequired = true
	common.QuotaForNewUser = 0
	common.QuotaPerUnit = 100
	constant.GenerateDefaultToken = false

	t.Cleanup(func() {
		common.RegisterEnabled = previousRegisterEnabled
		common.PasswordRegisterEnabled = previousPasswordRegisterEnabled
		common.EmailVerificationEnabled = previousEmailVerificationEnabled
		common.InviteCodeRequired = previousInviteCodeRequired
		common.QuotaForNewUser = previousQuotaForNewUser
		common.QuotaPerUnit = previousQuotaPerUnit
		constant.GenerateDefaultToken = previousGenerateDefaultToken
	})
}

func performPasswordRegistration(t *testing.T, username string, affCode string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"username": username,
		"password": "password123",
		"aff_code": affCode,
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/user/register", bytes.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	Register(context)
	return recorder
}

func cleanupRegistrationInviteUser(t *testing.T, username string) {
	t.Helper()
	var user model.User
	if err := model.DB.Unscoped().Where("username = ?", username).First(&user).Error; err != nil {
		return
	}
	model.DB.Where("invitee_id = ? OR inviter_id = ?", user.Id, user.Id).Delete(&model.InviteReward{})
	model.DB.Where("user_id = ?", user.Id).Delete(&model.UserOAuthBinding{})
	model.DB.Where("user_id = ?", user.Id).Delete(&model.Token{})
	model.DB.Where("user_id = ?", user.Id).Delete(&model.Log{})
	model.DB.Unscoped().Delete(&user)
}

func TestPasswordRegistrationRejectsMissingInviteWithoutCreatingUser(t *testing.T) {
	withRegistrationInviteTestSettings(t)
	username := fmt.Sprintf("mi%016x", uint64(time.Now().UnixNano()))
	t.Cleanup(func() { cleanupRegistrationInviteUser(t, username) })

	recorder := performPasswordRegistration(t, username, "")
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "\"success\":false")

	var count int64
	require.NoError(t, model.DB.Unscoped().Model(&model.User{}).
		Where("username = ?", username).Count(&count).Error)
	assert.Zero(t, count)
}

func TestPasswordRegistrationConsumesAdminCodeInUserTransaction(t *testing.T) {
	withRegistrationInviteTestSettings(t)
	suffix := uint64(time.Now().UnixNano())
	username := fmt.Sprintf("ai%016x", suffix)
	codeValue := fmt.Sprintf("ac%016x", suffix)
	code := model.InviteCode{
		Code:        codeValue,
		Status:      common.InviteCodeStatusEnabled,
		CreatorId:   1,
		CreatedTime: common.GetTimestamp(),
	}
	require.NoError(t, model.DB.Create(&code).Error)
	t.Cleanup(func() {
		cleanupRegistrationInviteUser(t, username)
		model.DB.Unscoped().Delete(&code)
	})

	recorder := performPasswordRegistration(t, username, codeValue)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "\"success\":true")

	var user model.User
	require.NoError(t, model.DB.Where("username = ?", username).First(&user).Error)
	require.NoError(t, model.DB.First(&code, code.Id).Error)
	assert.Equal(t, common.InviteCodeStatusUsed, code.Status)
	assert.Equal(t, user.Id, code.UsedUserId)
	assert.Zero(t, user.Quota)

	var rewards int64
	require.NoError(t, model.DB.Model(&model.InviteReward{}).
		Where("invitee_id = ?", user.Id).Count(&rewards).Error)
	assert.Zero(t, rewards)
}

func TestPasswordRegistrationPersonalCodeCreditsBothUsers(t *testing.T) {
	withRegistrationInviteTestSettings(t)
	suffix := uint64(time.Now().UnixNano())
	inviterName := fmt.Sprintf("pr%016x", suffix)
	inviteeName := fmt.Sprintf("pe%016x", suffix)
	inviter := model.User{
		Username:    inviterName,
		DisplayName: inviterName,
		AffCode:     fmt.Sprintf("pa%016x", suffix),
		Status:      common.UserStatusEnabled,
		Role:        common.RoleCommonUser,
	}
	require.NoError(t, model.DB.Create(&inviter).Error)
	t.Cleanup(func() {
		cleanupRegistrationInviteUser(t, inviteeName)
		cleanupRegistrationInviteUser(t, inviterName)
	})

	recorder := performPasswordRegistration(t, inviteeName, inviter.AffCode)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "\"success\":true")

	var paidInviter model.User
	var paidInvitee model.User
	require.NoError(t, model.DB.First(&paidInviter, inviter.Id).Error)
	require.NoError(t, model.DB.Where("username = ?", inviteeName).First(&paidInvitee).Error)
	assert.Equal(t, inviter.Id, paidInvitee.InviterId)
	assert.Equal(t, 500, paidInviter.Quota)
	assert.Equal(t, 500, paidInvitee.Quota)
	assert.Equal(t, 1, paidInviter.AffCount)

	var reward model.InviteReward
	require.NoError(t, model.DB.Where("invitee_id = ?", paidInvitee.Id).First(&reward).Error)
	assert.Equal(t, inviter.Id, reward.InviterId)
	assert.Equal(t, 1, reward.InviteCount)
}

func TestOAuthRegistrationEnforcesInviteGateButExistingLoginBypassesIt(t *testing.T) {
	withRegistrationInviteTestSettings(t)
	suffix := uint64(time.Now().UnixNano())
	newUsername := fmt.Sprintf("on%016x", suffix)
	t.Cleanup(func() { cleanupRegistrationInviteUser(t, newUsername) })

	provider := &registrationInviteOAuthProvider{}
	_, err := findOrCreateOAuthUser(provider, &oauth.OAuthUser{
		ProviderUserID: fmt.Sprintf("new-provider-%x", suffix),
		Username:       newUsername,
		DisplayName:    newUsername,
	}, "")
	require.ErrorIs(t, err, service.ErrRegistrationInviteInvalid)

	var newUserCount int64
	require.NoError(t, model.DB.Unscoped().Model(&model.User{}).
		Where("username = ?", newUsername).Count(&newUserCount).Error)
	assert.Zero(t, newUserCount)

	existingUsername := fmt.Sprintf("oe%016x", suffix)
	existing := model.User{
		Username:    existingUsername,
		DisplayName: existingUsername,
		AffCode:     fmt.Sprintf("oea%016x", suffix),
		GitHubId:    fmt.Sprintf("existing-provider-%x", suffix),
		Status:      common.UserStatusEnabled,
		Role:        common.RoleCommonUser,
	}
	require.NoError(t, model.DB.Create(&existing).Error)
	t.Cleanup(func() { cleanupRegistrationInviteUser(t, existingUsername) })

	provider.existingUserID = existing.Id
	loggedIn, err := findOrCreateOAuthUser(provider, &oauth.OAuthUser{
		ProviderUserID: existing.GitHubId,
	}, "")
	require.NoError(t, err)
	assert.Equal(t, existing.Id, loggedIn.Id)
}

func TestOAuthRegistrationPersonalCodeCreditsBothUsers(t *testing.T) {
	withRegistrationInviteTestSettings(t)
	suffix := uint64(time.Now().UnixNano())
	inviterName := fmt.Sprintf("or%016x", suffix)
	inviteeName := fmt.Sprintf("oi%016x", suffix)
	inviter := model.User{
		Username:    inviterName,
		DisplayName: inviterName,
		AffCode:     fmt.Sprintf("ora%016x", suffix),
		Status:      common.UserStatusEnabled,
		Role:        common.RoleCommonUser,
	}
	require.NoError(t, model.DB.Create(&inviter).Error)
	t.Cleanup(func() {
		cleanupRegistrationInviteUser(t, inviteeName)
		cleanupRegistrationInviteUser(t, inviterName)
	})

	created, err := findOrCreateOAuthUser(&registrationInviteOAuthProvider{}, &oauth.OAuthUser{
		ProviderUserID: fmt.Sprintf("personal-provider-%x", suffix),
		Username:       inviteeName,
		DisplayName:    inviteeName,
	}, inviter.AffCode)
	require.NoError(t, err)
	assert.Equal(t, inviter.Id, created.InviterId)

	var paidInviter model.User
	var paidInvitee model.User
	require.NoError(t, model.DB.First(&paidInviter, inviter.Id).Error)
	require.NoError(t, model.DB.First(&paidInvitee, created.Id).Error)
	assert.Equal(t, 500, paidInviter.Quota)
	assert.Equal(t, 500, paidInvitee.Quota)
}

func TestCustomOAuthRegistrationConsumesAdminCodeInSameTransaction(t *testing.T) {
	withRegistrationInviteTestSettings(t)
	suffix := uint64(time.Now().UnixNano())
	username := fmt.Sprintf("co%016x", suffix)
	codeValue := fmt.Sprintf("coa%016x", suffix)
	providerConfig := model.CustomOAuthProvider{
		Name:    "Invite test custom provider",
		Slug:    fmt.Sprintf("invite-test-%x", suffix),
		Enabled: true,
	}
	require.NoError(t, model.DB.Create(&providerConfig).Error)
	code := model.InviteCode{
		Code:        codeValue,
		Status:      common.InviteCodeStatusEnabled,
		CreatorId:   1,
		CreatedTime: common.GetTimestamp(),
	}
	require.NoError(t, model.DB.Create(&code).Error)
	t.Cleanup(func() {
		cleanupRegistrationInviteUser(t, username)
		model.DB.Unscoped().Delete(&code)
		model.DB.Unscoped().Delete(&providerConfig)
	})

	provider := oauth.NewGenericOAuthProvider(&providerConfig)
	created, err := findOrCreateOAuthUser(provider, &oauth.OAuthUser{
		ProviderUserID: fmt.Sprintf("custom-user-%x", suffix),
		Username:       username,
		DisplayName:    username,
	}, codeValue)
	require.NoError(t, err)

	require.NoError(t, model.DB.First(&code, code.Id).Error)
	assert.Equal(t, common.InviteCodeStatusUsed, code.Status)
	assert.Equal(t, created.Id, code.UsedUserId)
	var binding model.UserOAuthBinding
	require.NoError(t, model.DB.Where(
		"user_id = ? AND provider_id = ?", created.Id, providerConfig.Id,
	).First(&binding).Error)
}

func TestWeChatRegistrationEnforcesInviteGate(t *testing.T) {
	withRegistrationInviteTestSettings(t)
	wechatId := fmt.Sprintf("wechat-missing-%x", uint64(time.Now().UnixNano()))

	_, err := createWeChatRegistration(wechatId, "")
	require.ErrorIs(t, err, service.ErrRegistrationInviteInvalid)

	var count int64
	require.NoError(t, model.DB.Unscoped().Model(&model.User{}).
		Where("wechat_id = ?", wechatId).Count(&count).Error)
	assert.Zero(t, count)
}

func TestWeChatRegistrationPersonalCodeCreditsBothUsers(t *testing.T) {
	withRegistrationInviteTestSettings(t)
	suffix := uint64(time.Now().UnixNano())
	inviterName := fmt.Sprintf("wr%016x", suffix)
	inviter := model.User{
		Username:    inviterName,
		DisplayName: inviterName,
		AffCode:     fmt.Sprintf("wra%016x", suffix),
		Status:      common.UserStatusEnabled,
		Role:        common.RoleCommonUser,
	}
	require.NoError(t, model.DB.Create(&inviter).Error)
	var created *model.User
	t.Cleanup(func() {
		if created != nil {
			cleanupRegistrationInviteUser(t, created.Username)
		}
		cleanupRegistrationInviteUser(t, inviterName)
	})

	var err error
	created, err = createWeChatRegistration(
		fmt.Sprintf("wechat-personal-%x", suffix),
		inviter.AffCode,
	)
	require.NoError(t, err)
	assert.Equal(t, inviter.Id, created.InviterId)

	var paidInviter model.User
	var paidInvitee model.User
	require.NoError(t, model.DB.First(&paidInviter, inviter.Id).Error)
	require.NoError(t, model.DB.First(&paidInvitee, created.Id).Error)
	assert.Equal(t, 500, paidInviter.Quota)
	assert.Equal(t, 500, paidInvitee.Quota)
}
