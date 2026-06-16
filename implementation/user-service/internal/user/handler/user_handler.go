package handler

import (
	"net/http"

	pkgerrors "github.com/emzhofb/gowallet/pkg/errors"
	"github.com/emzhofb/gowallet/user-service/internal/user/service"
	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	svc service.UserService
}

func NewUserHandler(svc service.UserService) *UserHandler {
	return &UserHandler{svc: svc}
}

type RegisterRequest struct {
	Name     string `json:"name" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

func (h *UserHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	user, err := h.svc.Register(req.Name, req.Email, req.Password)
	if err != nil {
		status, resp := pkgerrors.FormatErrorResponse(err)
		c.JSON(status, resp)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    user,
	})
}

func (h *UserHandler) GetProfile(c *gin.Context) {
	id := c.Param("id")
	user, err := h.svc.GetProfile(id)
	if err != nil {
		status, resp := pkgerrors.FormatErrorResponse(err)
		c.JSON(status, resp)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    user,
	})
}
