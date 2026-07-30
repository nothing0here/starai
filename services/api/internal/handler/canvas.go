package handler

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/starai/api/internal/service"
	"github.com/starai/api/internal/util"
)

func (h *Handler) CreateCanvas(c *gin.Context) {
	var input service.SaveCanvasInput
	if err := c.ShouldBindJSON(&input); err != nil {
		util.BadRequest(c, "请求参数错误")
		return
	}
	item, err := h.canvases.Create(c.Request.Context(), c.GetInt64("user_id"), input)
	if err != nil {
		util.BadRequest(c, err.Error())
		return
	}
	util.OK(c, item)
}

func (h *Handler) ListCanvases(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "30"))
	items, total, err := h.canvases.List(c.Request.Context(), c.GetInt64("user_id"), page, pageSize)
	if err != nil {
		util.InternalError(c, err.Error())
		return
	}
	util.OK(c, map[string]interface{}{"items": items, "total": total})
}

func (h *Handler) GetCanvas(c *gin.Context) {
	item, err := h.canvases.Get(c.Request.Context(), c.GetInt64("user_id"), c.Param("id"))
	if errors.Is(err, pgx.ErrNoRows) {
		util.NotFound(c, "画布不存在")
		return
	}
	if err != nil {
		util.InternalError(c, err.Error())
		return
	}
	util.OK(c, item)
}

func (h *Handler) UpdateCanvas(c *gin.Context) {
	var input service.SaveCanvasInput
	if err := c.ShouldBindJSON(&input); err != nil {
		util.BadRequest(c, "请求参数错误")
		return
	}
	item, err := h.canvases.Update(c.Request.Context(), c.GetInt64("user_id"), c.Param("id"), input)
	if errors.Is(err, pgx.ErrNoRows) {
		util.NotFound(c, "画布不存在")
		return
	}
	if err != nil {
		util.BadRequest(c, err.Error())
		return
	}
	util.OK(c, item)
}

func (h *Handler) DeleteCanvas(c *gin.Context) {
	err := h.canvases.Delete(c.Request.Context(), c.GetInt64("user_id"), c.Param("id"))
	if errors.Is(err, pgx.ErrNoRows) {
		util.NotFound(c, "画布不存在")
		return
	}
	if err != nil {
		util.InternalError(c, err.Error())
		return
	}
	util.OK(c, nil)
}
