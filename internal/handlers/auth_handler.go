package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/fulltank-garage/fulltankgarage-api/internal/httpx"
	"github.com/fulltank-garage/fulltankgarage-api/internal/services"
)

type AuthHandler struct {
	memberService *services.MemberService
	authService   *services.AuthService
}

func NewAuthHandler(memberService *services.MemberService, authService *services.AuthService) *AuthHandler {
	return &AuthHandler{
		memberService: memberService,
		authService:   authService,
	}
}

func (h *AuthHandler) Register(c *gin.Context) {
	image, err := c.FormFile("storefrontImage")
	if err != nil {
		httpx.BadRequest(c, "กรุณาอัปโหลดรูปหน้าร้าน")
		return
	}

	member, err := h.memberService.Register(c.Request.Context(), services.RegisterMemberInput{
		FirstName:       c.PostForm("firstName"),
		LastName:        c.PostForm("lastName"),
		Nickname:        c.PostForm("nickname"),
		Phone:           c.PostForm("phone"),
		CitizenID:       c.PostForm("citizenId"),
		ShopPageURL:     c.PostForm("shopPageUrl"),
		StorefrontImage: image,
		LineUserID:      firstNonEmpty(c.PostForm("lineUserId"), c.PostForm("line_user_id"), c.GetHeader("X-Line-User-Id")),
		LineIDToken:     firstNonEmpty(c.PostForm("lineIdToken"), c.PostForm("line_id_token"), c.GetHeader("X-Line-ID-Token")),
		LineDisplayName: firstNonEmpty(c.PostForm("lineDisplayName"), c.PostForm("line_display_name"), c.GetHeader("X-Line-Display-Name")),
		LinePictureURL:  firstNonEmpty(c.PostForm("linePictureUrl"), c.PostForm("line_picture_url"), c.GetHeader("X-Line-Picture-Url")),
	})
	if err != nil {
		httpx.Error(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "ส่งข้อมูลสมัครสำเร็จ",
		"id":      member.ID,
		"data":    member,
	})
}

func (h *AuthHandler) Profile(c *gin.Context) {
	member, err := h.memberService.Profile(
		c.Request.Context(),
		firstNonEmpty(c.Query("lineUserId"), c.Query("line_user_id"), c.GetHeader("X-Line-User-Id")),
		firstNonEmpty(c.Query("lineIdToken"), c.Query("line_id_token"), c.GetHeader("X-Line-ID-Token")),
		firstNonEmpty(c.Query("id"), c.Param("id")),
	)
	if err != nil {
		httpx.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, member)
}

func (h *AuthHandler) RegistrationStatus(c *gin.Context) {
	member, err := h.memberService.RegistrationStatus(
		c.Request.Context(),
		firstNonEmpty(c.Query("lineUserId"), c.Query("line_user_id"), c.GetHeader("X-Line-User-Id")),
		firstNonEmpty(c.Query("lineIdToken"), c.Query("line_id_token"), c.GetHeader("X-Line-ID-Token")),
	)
	if err != nil {
		httpx.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, member)
}

func (h *AuthHandler) SyncRichMenu(c *gin.Context) {
	result, err := h.memberService.SyncLineRichMenu(
		c.Request.Context(),
		firstNonEmpty(c.Query("lineUserId"), c.Query("line_user_id"), c.GetHeader("X-Line-User-Id")),
		firstNonEmpty(c.Query("lineIdToken"), c.Query("line_id_token"), c.GetHeader("X-Line-ID-Token")),
	)
	if err != nil {
		httpx.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *AuthHandler) PointWallet(c *gin.Context) {
	wallet, err := h.memberService.PointWallet(
		c.Request.Context(),
		firstNonEmpty(c.Query("lineUserId"), c.Query("line_user_id"), c.GetHeader("X-Line-User-Id")),
		firstNonEmpty(c.Query("lineIdToken"), c.Query("line_id_token"), c.GetHeader("X-Line-ID-Token")),
	)
	if err != nil {
		httpx.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, wallet)
}

func (h *AuthHandler) Login(c *gin.Context) {
	var input services.LoginInput
	if err := c.ShouldBindJSON(&input); err != nil {
		httpx.BadRequest(c, "รูปแบบข้อมูลเข้าสู่ระบบไม่ถูกต้อง")
		return
	}

	session, err := h.authService.Login(input)
	if err != nil {
		httpx.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, session)
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	var input services.RefreshInput
	if err := c.ShouldBindJSON(&input); err != nil {
		httpx.BadRequest(c, "รูปแบบข้อมูลต่ออายุ token ไม่ถูกต้อง")
		return
	}

	session, err := h.authService.Refresh(input)
	if err != nil {
		httpx.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, session)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}

	return ""
}
