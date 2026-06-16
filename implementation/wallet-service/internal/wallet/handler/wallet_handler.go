package handler

import (
	"net/http"

	pkgerrors "github.com/emzhofb/gowallet/pkg/errors"
	"github.com/emzhofb/gowallet/wallet-service/internal/wallet/service"
	"github.com/gin-gonic/gin"
)

type WalletHandler struct {
	svc service.WalletService
}

func NewWalletHandler(svc service.WalletService) *WalletHandler {
	return &WalletHandler{svc: svc}
}

func (h *WalletHandler) Create(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		status, resp := pkgerrors.FormatErrorResponse(pkgerrors.NewAppError(http.StatusUnauthorized, "UNAUTHORIZED", "User identity header missing"))
		c.JSON(status, resp)
		return
	}

	wallet, err := h.svc.CreateWallet(c.Request.Context(), userID)
	if err != nil {
		status, resp := pkgerrors.FormatErrorResponse(err)
		c.JSON(status, resp)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    wallet,
	})
}

func (h *WalletHandler) GetBalance(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		status, resp := pkgerrors.FormatErrorResponse(pkgerrors.NewAppError(http.StatusUnauthorized, "UNAUTHORIZED", "User identity header missing"))
		c.JSON(status, resp)
		return
	}

	wallet, err := h.svc.GetByUserID(c.Request.Context(), userID)
	if err != nil {
		status, resp := pkgerrors.FormatErrorResponse(err)
		c.JSON(status, resp)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"wallet_id": wallet.ID,
			"balance":   wallet.Balance,
		},
	})
}
