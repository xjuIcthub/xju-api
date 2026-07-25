package controller

import (
	"math"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

type defaultPoolPricingRequest struct {
	Multiplier float64 `json:"multiplier"`
}

func GetDefaultPoolPricing(c *gin.Context) {
	common.ApiSuccess(c, gin.H{
		"multiplier": service.GetDefaultPoolPricingMultiplier(),
	})
}

func UpdateDefaultPoolPricing(c *gin.Context) {
	req := defaultPoolPricingRequest{}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "无效的倍率参数")
		return
	}
	if math.IsNaN(req.Multiplier) || math.IsInf(req.Multiplier, 0) || req.Multiplier < 0.1 || req.Multiplier > 100 {
		common.ApiErrorMsg(c, "Default 池倍率必须在 0.1 到 100 之间")
		return
	}
	previous := service.GetDefaultPoolPricingMultiplier()
	if err := service.SetDefaultPoolPricingMultiplier(req.Multiplier); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "pool.default_pricing.update", map[string]interface{}{
		"from": previous,
		"to":   req.Multiplier,
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"multiplier": service.GetDefaultPoolPricingMultiplier(),
		},
	})
}
