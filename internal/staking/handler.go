package staking

import (
	"net/http"

	"github.com/cristianrisueo/gopay/pkg/response"
	"github.com/gin-gonic/gin"
)

// StakingHandler handles HTTP requests for staking operations
type StakingHandler struct {
	stakingService *StakingService
}

// NewStakingHandler creates a new StakingHandler
func NewStakingHandler(stakingService *StakingService) *StakingHandler {
	return &StakingHandler{
		stakingService: stakingService,
	}
}

// CreateStake handles a stake position reation
func (sh *StakingHandler) CreateStake(ctx *gin.Context) {
	// Gest the user ID from JWT context
	userID, exists := ctx.Get("userID")
	if !exists {
		response.Unauthorized(ctx, "User not authenticated")
		return
	}

	// Parses the request body in a CreateStakeRequest struct
	var req CreateStakeRequest

	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.BadRequest(ctx, "Invalid request body")
		return
	}

	// Calls the service to create stake
	stakeResponse, err := sh.stakingService.CreateStake(ctx.Request.Context(), userID.(string), req.Amount, req.Currency)
	if err != nil {
		response.HandleStakingError(ctx, err)
		return
	}

	response.Success(ctx, http.StatusCreated, stakeResponse)
}

// IncreaseStake handles a stake position increment (user adds more stake to his position)
func (sh *StakingHandler) IncreaseStake(ctx *gin.Context) {
	// Gets the user ID from JWT context
	userID, exists := ctx.Get("userID")
	if !exists {
		response.Unauthorized(ctx, "User not authenticated")
		return
	}

	// Gets the position ID from URL parameter
	positionID := ctx.Param("id")
	if positionID == "" {
		response.BadRequest(ctx, "Position ID is required")
		return
	}

	// Parses the request body in a IncreaseStakeRequest struct
	var req IncreaseStakeRequest

	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.BadRequest(ctx, "Invalid request body")
		return
	}

	// Calls the service to increase stake
	stakeResponse, err := sh.stakingService.IncreaseStake(ctx.Request.Context(), userID.(string), positionID, req.Amount)
	if err != nil {
		response.HandleStakingError(ctx, err)
		return
	}

	response.Success(ctx, http.StatusOK, stakeResponse)
}

// GetPositions handles the retrieval of every user position optionally filtered by status
func (sh *StakingHandler) GetPositions(ctx *gin.Context) {
	// Get user ID from JWT context
	userID, exists := ctx.Get("userID")
	if !exists {
		response.Unauthorized(ctx, "User not authenticated")
		return
	}

	// Reads optional status query parameter
	statusStr := ctx.Query("status")

	var status *PositionStatus
	if statusStr != "" {
		// Validates status is one of the valid values
		s := PositionStatus(statusStr)

		switch s {
		case StatusActive, StatusCompleted, StatusWithdrawn:
			status = &s
		default:
			response.BadRequest(ctx, "Invalid status: must be ACTIVE, COMPLETED, or WITHDRAWN")
			return
		}
	}

	// Call service to get user positions with optional status filter
	positions, err := sh.stakingService.GetUserPositions(ctx.Request.Context(), userID.(string), status)
	if err != nil {
		response.InternalServerError(ctx, "Failed to retrieve positions")
		return
	}

	response.Success(ctx, http.StatusOK, positions)
}

// WithdrawStake handles the stake withdrawal
func (sh *StakingHandler) WithdrawStake(ctx *gin.Context) {
	// Get user ID from JWT context
	userID, exists := ctx.Get("userID")
	if !exists {
		response.Unauthorized(ctx, "User not authenticated")
		return
	}

	// Get position ID from URL parameter
	positionID := ctx.Param("id")
	if positionID == "" {
		response.BadRequest(ctx, "Position ID is required")
		return
	}

	// Call service to withdraw stake
	withdrawResponse, err := sh.stakingService.WithdrawStake(ctx.Request.Context(), userID.(string), positionID)
	if err != nil {
		response.HandleStakingError(ctx, err)
		return
	}

	response.Success(ctx, http.StatusOK, withdrawResponse)
}
