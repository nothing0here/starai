package handler

import (
	"strings"
	"testing"
)

func TestValidateCustomerServiceConfig(t *testing.T) {
	tests := []struct {
		name string
		req  map[string]interface{}
		want string
	}{
		{name: "builtin", req: map[string]interface{}{"customer_service_mode": "builtin"}},
		{name: "custom script", req: map[string]interface{}{"customer_service_mode": "custom_script", "customer_service_custom_script": "<script>window.chat = true;</script>"}},
		{name: "invalid mode", req: map[string]interface{}{"customer_service_mode": "other"}, want: "首页客服方式参数错误"},
		{name: "missing script tag", req: map[string]interface{}{"customer_service_custom_script": "window.chat = true;"}, want: "第三方客服脚本必须包含完整的 <script> 标签"},
		{name: "too large", req: map[string]interface{}{"customer_service_custom_script": "<script>" + strings.Repeat("x", 100*1024) + "</script>"}, want: "第三方客服脚本不能超过 100KB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateCustomerServiceConfig(tt.req); got != tt.want {
				t.Fatalf("validateCustomerServiceConfig() = %q, want %q", got, tt.want)
			}
		})
	}
}
