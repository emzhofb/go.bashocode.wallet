package handler

import (
	"net/http"
	"strings"

	pkgerrors "github.com/emzhofb/gowallet/pkg/errors"
	"github.com/emzhofb/gowallet/auth-service/internal/auth/service"
	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	svc service.AuthService
}

func NewAuthHandler(svc service.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	accessToken, refreshToken, err := h.svc.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		status, resp := pkgerrors.FormatErrorResponse(err)
		c.JSON(status, resp)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"access_token":  accessToken,
			"refresh_token": refreshToken,
		},
	})
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	accessToken, refreshToken, err := h.svc.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		status, resp := pkgerrors.FormatErrorResponse(err)
		c.JSON(status, resp)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"access_token":  accessToken,
			"refresh_token": refreshToken,
		},
	})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	authHeader := c.GetHeader("Authorization")
	token := strings.TrimPrefix(authHeader, "Bearer ")

	err := h.svc.Logout(c.Request.Context(), token, req.RefreshToken)
	if err != nil {
		status, resp := pkgerrors.FormatErrorResponse(err)
		c.JSON(status, resp)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "logged out successfully",
	})
}
