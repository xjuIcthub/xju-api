package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestParseQuotaProvider(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name     string
		query    string
		provider string
		ok       bool
	}{
		{name: "legacy default remains Codex", provider: common.PoolProviderCodex, ok: true},
		{name: "all aggregates both providers", query: "?provider=ALL", provider: common.PoolProviderAll, ok: true},
		{name: "Claude is normalized", query: "?provider=Claude", provider: common.PoolProviderClaude, ok: true},
		{name: "unknown provider is rejected", query: "?provider=gemini", ok: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("GET", "/api/data"+tc.query, nil)

			provider, ok := parseQuotaProvider(c)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.provider, provider)
		})
	}
}
