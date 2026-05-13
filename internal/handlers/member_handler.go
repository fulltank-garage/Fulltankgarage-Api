package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/fulltank-garage/fulltankgarage-api/internal/httpx"
	"github.com/fulltank-garage/fulltankgarage-api/internal/services"
)

type MemberHandler struct {
	memberService *services.MemberService
}

func NewMemberHandler(memberService *services.MemberService) *MemberHandler {
	return &MemberHandler{memberService: memberService}
}

func (h *MemberHandler) ListApplications(c *gin.Context) {
	members, err := h.memberService.ListApplications(c.Request.Context())
	if err != nil {
		httpx.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, members)
}

func (h *MemberHandler) ListMembers(c *gin.Context) {
	members, err := h.memberService.ListMembers(c.Request.Context())
	if err != nil {
		httpx.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, members)
}

func (h *MemberHandler) UpdateStatus(c *gin.Context) {
	var input struct {
		Status           string   `json:"status"`
		RejectionReasons []string `json:"rejectionReasons"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		httpx.BadRequest(c, "รูปแบบข้อมูลสถานะไม่ถูกต้อง")
		return
	}

	member, err := h.memberService.UpdateApplicationStatus(c.Request.Context(), c.Param("id"), input.Status, input.RejectionReasons)
	if err != nil {
		httpx.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, member)
}

func (h *MemberHandler) DeleteMember(c *gin.Context) {
	if err := h.memberService.DeleteMember(c.Request.Context(), c.Param("id")); err != nil {
		httpx.Error(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *MemberHandler) DeleteApplication(c *gin.Context) {
	if err := h.memberService.DeleteApplication(c.Request.Context(), c.Param("id")); err != nil {
		httpx.Error(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}
