package pool

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/cristianrisueo/gopay/pkg/response"

	"github.com/gin-gonic/gin"
)

// PoolHandler handles HTTP requests for pool operations
type PoolHandler struct {
	poolService *PoolService
}

// NewPoolHandler creates a new pool handler
func NewPoolHandler(poolService *PoolService) *PoolHandler {
	return &PoolHandler{poolService: poolService}
}

// GetAllPoolStats retrieves statistics for all pools
func (ph *PoolHandler) GetAllPoolStats(ctx *gin.Context) {
	allStats, err := ph.poolService.GetAllPoolsStats(ctx.Request.Context())
	if err != nil {
		response.InternalServerError(ctx, "Failed to retrieve pool statistics")
		return
	}

	response.Success(ctx, http.StatusOK, allStats)
}

// GetPoolStats retrieves statistics for a specific pool by currency
func (ph *PoolHandler) GetPoolStats(ctx *gin.Context) {
	currency := ctx.Param("currency")

	// Validates the currency received by param
	poolCurrency := PoolCurrency(currency)
	if !poolCurrency.IsValid() {
		response.BadRequest(ctx, "Invalid currency. Supported: EUR, USD, JPY")
		return
	}

	// Gets the pool stats from the service
	stats, err := ph.poolService.GetPoolStatsByCurrency(ctx.Request.Context(), poolCurrency)
	if err != nil {
		if errors.Is(err, ErrPoolNotFound) {
			response.NotFound(ctx, "Pool not found")
			return
		}

		response.InternalServerError(ctx, "Failed to retrieve pool statistics")
		return
	}

	response.Success(ctx, http.StatusOK, stats)
}

// GetInterestHistory retrieves interest distribution history for a pool
func (ph *PoolHandler) GetInterestHistory(ctx *gin.Context) {
	currency := ctx.Param("currency")
	daysParam := ctx.DefaultQuery("days", "30")

	// Validates the currency
	poolCurrency := PoolCurrency(currency)
	if !poolCurrency.IsValid() {
		response.BadRequest(ctx, "Invalid currency. Supported: EUR, USD, JPY")
		return
	}

	// Parses the days to int
	days, err := strconv.Atoi(daysParam)
	if err != nil || days <= 0 {
		response.BadRequest(ctx, "Invalid days parameter. Must be a positive integer")
		return
	}

	// Gets the interest history from the service
	distributions, err := ph.poolService.GetInterestHistoryByCurrency(ctx.Request.Context(), poolCurrency, days)
	if err != nil {
		if errors.Is(err, ErrPoolNotFound) {
			response.NotFound(ctx, "Pool not found")
			return
		}

		response.InternalServerError(ctx, "Failed to retrieve interest history")
		return
	}

	// Casts to InterestDistributionResponse response format
	var respDistributions []InterestDistributionResponse

	for _, dist := range distributions {
		resp := InterestDistributionResponse{
			CreatedAt:      dist.CreatedAt.Format("2006-01-02 15:04:05"),
			InterestAmount: dist.InterestAmount.String(),
			APYRate:        dist.APYRate.String(),
		}

		respDistributions = append(respDistributions, resp)
	}

	response.Success(ctx, http.StatusOK, respDistributions)
}
