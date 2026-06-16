package handler

import (
	"net/http"

	pkgerrors "github.com/emzhofb/gowallet/pkg/errors"
	"github.com/emzhofb/gowallet/transaction-service/internal/transaction/model"
	"github.com/emzhofb/gowallet/transaction-service/internal/transaction/service"
	"github.com/gin-gonic/gin"
)

type TransactionHandler struct {
	svc service.TransactionService
}

func NewTransactionHandler(svc service.TransactionService) *TransactionHandler {
	return &TransactionHandler{svc: svc}
}

func (h *TransactionHandler) Transfer(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		status, resp := pkgerrors.FormatErrorResponse(pkgerrors.NewAppError(http.StatusUnauthorized, "UNAUTHORIZED", "User identity header missing"))
		c.JSON(status, resp)
		return
	}

	var req model.TransferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	tx, err := h.svc.Transfer(c.Request.Context(), userID, req)
	if err != nil {
		status, resp := pkgerrors.FormatErrorResponse(err)
		c.JSON(status, resp)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    tx,
	})
}
