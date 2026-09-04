package middleware

import (
	"strings"
	"testing"
)

func TestPrometheusTextIncludesCoreMetrics(t *testing.T) {
	text := PrometheusText(15)
	for _, name := range []string{"starai_http_requests_total", "starai_http_errors_total", "starai_payment_webhook_rejected_total", "starai_content_safety_blocked_total", "starai_creative_agent_plan_requests_total", "starai_creative_agent_contract_failures_total", "starai_creative_agent_clarifications_total", "starai_creative_agent_submissions_total", "starai_worker_heartbeat_age_seconds 15"} {
		if !strings.Contains(text, name) {
			t.Fatalf("metrics output missing %q", name)
		}
	}
}
