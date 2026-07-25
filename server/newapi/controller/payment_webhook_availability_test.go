package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func confirmPaymentComplianceForTest(t *testing.T) {
	t.Helper()
	paymentSetting := operation_setting.GetPaymentSetting()
	originalConfirmed := paymentSetting.ComplianceConfirmed
	originalTermsVersion := paymentSetting.ComplianceTermsVersion
	originalOnlinePaymentEnabled := paymentSetting.OnlinePaymentEnabled
	t.Cleanup(func() {
		paymentSetting.ComplianceConfirmed = originalConfirmed
		paymentSetting.ComplianceTermsVersion = originalTermsVersion
		paymentSetting.OnlinePaymentEnabled = originalOnlinePaymentEnabled
	})
	paymentSetting.ComplianceConfirmed = true
	paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	paymentSetting.OnlinePaymentEnabled = true
}

func disableOnlinePaymentForTest(t *testing.T) {
	t.Helper()
	paymentSetting := operation_setting.GetPaymentSetting()
	originalOnlinePaymentEnabled := paymentSetting.OnlinePaymentEnabled
	t.Cleanup(func() {
		paymentSetting.OnlinePaymentEnabled = originalOnlinePaymentEnabled
	})
	paymentSetting.OnlinePaymentEnabled = false
}

func TestOnlinePaymentMasterSwitchOverridesConfiguredGateway(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	disableOnlinePaymentForTest(t)
	originalAPISecret := setting.StripeApiSecret
	originalWebhookSecret := setting.StripeWebhookSecret
	originalPriceID := setting.StripePriceId
	t.Cleanup(func() {
		setting.StripeApiSecret = originalAPISecret
		setting.StripeWebhookSecret = originalWebhookSecret
		setting.StripePriceId = originalPriceID
	})

	setting.StripeApiSecret = "sk_test_123"
	setting.StripeWebhookSecret = "whsec_test"
	setting.StripePriceId = "price_123"
	require.False(t, isStripeTopUpEnabled())
	require.False(t, isStripeWebhookEnabled())
}

func TestDirectOnlinePaymentHandlersRejectWhenMasterSwitchIsOff(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	disableOnlinePaymentForTest(t)
	gin.SetMode(gin.TestMode)

	testCases := []struct {
		name    string
		handler gin.HandlerFunc
		body    string
	}{
		{name: "epay amount", handler: RequestAmount, body: `{"amount":10}`},
		{name: "epay order", handler: RequestEpay, body: `{"amount":10,"payment_method":"alipay"}`},
		{name: "stripe amount", handler: RequestStripeAmount, body: `{"amount":10}`},
		{name: "stripe order", handler: RequestStripePay, body: `{"amount":10,"payment_method":"stripe"}`},
		{name: "creem order", handler: RequestCreemPay, body: `{"product_id":"prod_123","payment_method":"creem"}`},
		{name: "waffo amount", handler: RequestWaffoAmount, body: `{"amount":10}`},
		{name: "waffo order", handler: RequestWaffoPay, body: `{"amount":10}`},
		{name: "waffo pancake amount", handler: RequestWaffoPancakeAmount, body: `{"amount":10}`},
		{name: "waffo pancake order", handler: RequestWaffoPancakePay, body: `{"amount":10}`},
		{name: "subscription epay", handler: SubscriptionRequestEpay, body: `{"plan_id":1,"payment_method":"alipay"}`},
		{name: "subscription stripe", handler: SubscriptionRequestStripePay, body: `{"plan_id":1}`},
		{name: "subscription creem", handler: SubscriptionRequestCreemPay, body: `{"plan_id":1}`},
		{name: "subscription waffo pancake", handler: SubscriptionRequestWaffoPancakePay, body: `{"plan_id":1}`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(tc.body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set("id", 1)

			tc.handler(c)

			var response struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
			require.False(t, response.Success)
			require.Equal(t, "在线支付功能暂未开放", response.Message)
		})
	}
}

func TestTopUpInfoKeepsRedemptionAvailableWhenOnlinePaymentIsOff(t *testing.T) {
	disableOnlinePaymentForTest(t)
	paymentSetting := operation_setting.GetPaymentSetting()
	originalConfirmed := paymentSetting.ComplianceConfirmed
	originalTermsVersion := paymentSetting.ComplianceTermsVersion
	t.Cleanup(func() {
		paymentSetting.ComplianceConfirmed = originalConfirmed
		paymentSetting.ComplianceTermsVersion = originalTermsVersion
	})
	paymentSetting.ComplianceConfirmed = false
	paymentSetting.ComplianceTermsVersion = ""

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/user/topup/info", nil)
	GetTopUpInfo(c)

	var response struct {
		Success bool `json:"success"`
		Data    struct {
			EnableRedemption     bool                `json:"enable_redemption"`
			OnlinePaymentEnabled bool                `json:"online_payment_enabled"`
			EnableOnlineTopUp    bool                `json:"enable_online_topup"`
			PayMethods           []map[string]string `json:"pay_methods"`
			TopUpLink            string              `json:"topup_link"`
			CreemProducts        string              `json:"creem_products"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.True(t, response.Data.EnableRedemption)
	require.False(t, response.Data.OnlinePaymentEnabled)
	require.False(t, response.Data.EnableOnlineTopUp)
	require.Empty(t, response.Data.PayMethods)
	require.Empty(t, response.Data.TopUpLink)
	require.Empty(t, response.Data.CreemProducts)
}

func TestStripeWebhookEnabledRequiresTopUpAndWebhookConfig(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalAPISecret := setting.StripeApiSecret
	originalWebhookSecret := setting.StripeWebhookSecret
	originalPriceID := setting.StripePriceId
	t.Cleanup(func() {
		setting.StripeApiSecret = originalAPISecret
		setting.StripeWebhookSecret = originalWebhookSecret
		setting.StripePriceId = originalPriceID
	})

	setting.StripeWebhookSecret = ""
	setting.StripeApiSecret = "sk_test_123"
	setting.StripePriceId = "price_123"
	require.False(t, isStripeWebhookEnabled())

	setting.StripeWebhookSecret = "whsec_test"
	require.True(t, isStripeWebhookEnabled())

	setting.StripePriceId = ""
	require.False(t, isStripeWebhookEnabled())
}

func TestCreemWebhookEnabledRequiresTopUpAndWebhookConfig(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalAPIKey := setting.CreemApiKey
	originalProducts := setting.CreemProducts
	originalWebhookSecret := setting.CreemWebhookSecret
	t.Cleanup(func() {
		setting.CreemApiKey = originalAPIKey
		setting.CreemProducts = originalProducts
		setting.CreemWebhookSecret = originalWebhookSecret
	})

	setting.CreemWebhookSecret = ""
	setting.CreemApiKey = "creem_api_key"
	setting.CreemProducts = `[{"productId":"prod_123"}]`
	require.False(t, isCreemWebhookEnabled())

	setting.CreemWebhookSecret = "creem_secret"
	require.True(t, isCreemWebhookEnabled())

	setting.CreemProducts = "[]"
	require.False(t, isCreemWebhookEnabled())
}

func TestWaffoWebhookEnabledRequiresTopUpAndWebhookConfig(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalEnabled := setting.WaffoEnabled
	originalSandbox := setting.WaffoSandbox
	originalAPIKey := setting.WaffoApiKey
	originalPrivateKey := setting.WaffoPrivateKey
	originalPublicCert := setting.WaffoPublicCert
	originalSandboxAPIKey := setting.WaffoSandboxApiKey
	originalSandboxPrivateKey := setting.WaffoSandboxPrivateKey
	originalSandboxPublicCert := setting.WaffoSandboxPublicCert
	t.Cleanup(func() {
		setting.WaffoEnabled = originalEnabled
		setting.WaffoSandbox = originalSandbox
		setting.WaffoApiKey = originalAPIKey
		setting.WaffoPrivateKey = originalPrivateKey
		setting.WaffoPublicCert = originalPublicCert
		setting.WaffoSandboxApiKey = originalSandboxAPIKey
		setting.WaffoSandboxPrivateKey = originalSandboxPrivateKey
		setting.WaffoSandboxPublicCert = originalSandboxPublicCert
	})

	setting.WaffoEnabled = true
	setting.WaffoSandbox = false
	setting.WaffoApiKey = ""
	setting.WaffoPrivateKey = "private"
	setting.WaffoPublicCert = "public"
	require.False(t, isWaffoWebhookEnabled())

	setting.WaffoApiKey = "api"
	require.True(t, isWaffoWebhookEnabled())

	setting.WaffoEnabled = false
	require.False(t, isWaffoWebhookEnabled())

	setting.WaffoEnabled = true
	setting.WaffoSandbox = true
	setting.WaffoSandboxApiKey = ""
	setting.WaffoSandboxPrivateKey = "sandbox_private"
	setting.WaffoSandboxPublicCert = "sandbox_public"
	require.False(t, isWaffoWebhookEnabled())

	setting.WaffoSandboxApiKey = "sandbox_api"
	require.True(t, isWaffoWebhookEnabled())
}

func TestWaffoPancakeWebhookEnabledRequiresTopUpAndWebhookConfig(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalMerchantID := setting.WaffoPancakeMerchantID
	originalPrivateKey := setting.WaffoPancakePrivateKey
	originalProductID := setting.WaffoPancakeProductID
	t.Cleanup(func() {
		setting.WaffoPancakeMerchantID = originalMerchantID
		setting.WaffoPancakePrivateKey = originalPrivateKey
		setting.WaffoPancakeProductID = originalProductID
	})

	// Presence of all three credentials enables the gateway. Webhook public
	// keys are bundled in the SDK and there is no separate Enabled toggle —
	// clear any of the three fields to disable.
	setting.WaffoPancakeMerchantID = ""
	setting.WaffoPancakePrivateKey = "private"
	setting.WaffoPancakeProductID = "product"
	require.False(t, isWaffoPancakeWebhookEnabled())

	setting.WaffoPancakeMerchantID = "merchant"
	require.True(t, isWaffoPancakeWebhookEnabled())

	setting.WaffoPancakeProductID = ""
	require.False(t, isWaffoPancakeWebhookEnabled())

	setting.WaffoPancakeProductID = "product"
	setting.WaffoPancakePrivateKey = ""
	require.False(t, isWaffoPancakeWebhookEnabled())
}

func TestEpayWebhookEnabledRequiresTopUpAndWebhookConfig(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalPayAddress := operation_setting.PayAddress
	originalEpayID := operation_setting.EpayId
	originalEpayKey := operation_setting.EpayKey
	originalPayMethods := operation_setting.PayMethods
	t.Cleanup(func() {
		operation_setting.PayAddress = originalPayAddress
		operation_setting.EpayId = originalEpayID
		operation_setting.EpayKey = originalEpayKey
		operation_setting.PayMethods = originalPayMethods
	})

	operation_setting.PayAddress = "https://pay.example.com"
	operation_setting.EpayId = "epay_id"
	operation_setting.EpayKey = ""
	operation_setting.PayMethods = []map[string]string{{"type": "alipay"}}
	require.False(t, isEpayWebhookEnabled())

	operation_setting.EpayKey = "epay_key"
	require.True(t, isEpayWebhookEnabled())

	operation_setting.PayMethods = nil
	require.False(t, isEpayWebhookEnabled())
}
