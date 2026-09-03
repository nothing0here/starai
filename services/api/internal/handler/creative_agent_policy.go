package handler

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/starai/api/internal/service"
	"github.com/starai/api/internal/util"
	"io"
)

func (h *Handler) AdminAgentPolicy(c *gin.Context) {
	if c.Param("code") != "general_creative_agent" {
		util.BadRequest(c, "仅通用 Agent 支持该策略")
		return
	}
	if c.Request.Method == "GET" {
		state, err := h.agents.GetAgentPolicy(c.Request.Context())
		if err != nil {
			util.BadRequest(c, err.Error())
			return
		}
		util.OK(c, state)
		return
	}
	var req struct {
		BaseVersion int64               `json:"base_version"`
		Policy      service.AgentPolicy `json:"policy"`
		Rollback    *int64              `json:"rollback_version"`
	}
	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, 131073))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&req) != nil {
		util.BadRequest(c, "策略参数格式错误或包含不允许修改的字段")
		return
	}
	var extra interface{}
	if decoder.Decode(&extra) != io.EOF {
		util.BadRequest(c, "策略请求必须是单个JSON对象")
		return
	}
	state, err := h.agents.SaveAgentPolicy(c.Request.Context(), req.BaseVersion, req.Policy, req.Rollback)
	if err != nil {
		util.BadRequest(c, err.Error())
		return
	}
	util.OK(c, state)
}
