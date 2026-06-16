package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"

	"github.com/emzhofb/gowallet/pkg/config"
	pkgerrors "github.com/emzhofb/gowallet/pkg/errors"
	"github.com/emzhofb/gowallet/pkg/rabbitmq"
	"github.com/gin-gonic/gin"
)

type PaymentHandler struct {
	rmq *rabbitmq.RabbitMQ
	cfg *config.Config
}

func NewPaymentHandler(rmq *rabbitmq.RabbitMQ, cfg *config.Config) *PaymentHandler {
	return &PaymentHandler{
		rmq: rmq,
		cfg: cfg,
	}
}

type PaymentWebhookRequest struct {
	UserID    string `json:"user_id" binding:"required"`
	Amount    int64  `json:"amount" binding:"required"`
	Reference string `json:"reference" binding:"required"`
	Type      string `json:"type" binding:"required"` // "topup" or "withdrawal"
}

func (h *PaymentHandler) Webhook(c *gin.Context) {
	// 1. Read Request Body
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "failed to read request body"})
		return
	}

	// 2. Validate Signature
	signature := c.GetHeader("X-Signature")
	if !h.verifySignature(bodyBytes, signature) {
		status, resp := pkgerrors.FormatErrorResponse(pkgerrors.NewAppError(http.StatusUnauthorized, "UNAUTHORIZED", "Invalid webhook signature"))
		c.JSON(status, resp)
		return
	}

	// 3. Unmarshal
	var req PaymentWebhookRequest
	err = json.Unmarshal(bodyBytes, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "failed to parse webhook payload"})
		return
	}

	// 4. Publish Event to RabbitMQ
	eventPayload := map[string]interface{}{
		"user_id":   req.UserID,
		"amount":    req.Amount,
		"reference": req.Reference,
		"type":      req.Type,
	}

	err = h.rmq.Publish(c.Request.Context(), "gowallet.events", "payment.completed", eventPayload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to dispatch payment event"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "payment webhook processed and dispatched",
	})
}

func (h *PaymentHandler) verifySignature(body []byte, signature string) bool {
	mac := hmac.New(sha256.New, []byte(h.cfg.WebhookSecret))
	mac.Write(body)
	expectedSig := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(signature), []byte(expectedSig))
}
