package alert

import (
	"fmt"
	"time"
)

// Type identifies the alert category.
type Type string

// Alert types.
const (
	TypeBudgetThreshold Type = "budget_threshold"
	TypeBudgetExceeded  Type = "budget_exceeded"
	TypeRunawayDetected Type = "runaway_detected"
	TypeErrorSpike      Type = "error_spike"
	TypeAgentKilled     Type = "agent_killed"
)

// Alert is a routed notification event.
//
//nolint:govet // keep alert payload fields grouped for readability/stability.
type Alert struct {
	Message         string         `json:"message"`
	Severity        string         `json:"severity"`
	Agent           string         `json:"agent"`
	Action          string         `json:"action,omitempty"`
	ID              string         `json:"id,omitempty"`
	Type            Type           `json:"type"`
	Timestamp       time.Time      `json:"timestamp"`
	ThresholdPct    float64        `json:"threshold_pct,omitempty"`
	SpentUSD        float64        `json:"spent_usd,omitempty"`
	LimitUSD        float64        `json:"limit_usd,omitempty"`
	PeriodStartedAt time.Time      `json:"period_started_at,omitempty"`
	Data            map[string]any `json:"data,omitempty"`
}

// NewBudgetThresholdAlert creates a budget-threshold alert event.
func NewBudgetThresholdAlert(agent string, thresholdPct float64, spentUSD float64, limitUSD float64, action string, periodStartedAt time.Time) Alert {
	return Alert{
		Type:            TypeBudgetThreshold,
		Agent:           agent,
		Severity:        "warning",
		Action:          action,
		ThresholdPct:    thresholdPct,
		SpentUSD:        spentUSD,
		LimitUSD:        limitUSD,
		PeriodStartedAt: periodStartedAt.UTC(),
		Timestamp:       time.Now().UTC(),
		Message: fmt.Sprintf(
			"Agent '%s' reached %.0f%% of budget ($%.2f / $%.2f)",
			agent,
			thresholdPct,
			spentUSD,
			limitUSD,
		),
	}
}

// NewBudgetExceededAlert creates a budget-exceeded alert event.
func NewBudgetExceededAlert(agent string, spentUSD float64, limitUSD float64, action string) Alert {
	return Alert{
		Type:      TypeBudgetExceeded,
		Agent:     agent,
		Severity:  "error",
		Action:    action,
		SpentUSD:  spentUSD,
		LimitUSD:  limitUSD,
		Timestamp: time.Now().UTC(),
		Message: fmt.Sprintf(
			"Agent '%s' exceeded budget ($%.2f / $%.2f)",
			agent,
			spentUSD,
			limitUSD,
		),
	}
}

// NewRunawayDetectedAlert creates a runaway-detection alert event.
func NewRunawayDetectedAlert(agent string, requestCount int, windowSeconds int) Alert {
	return Alert{
		Type:      TypeRunawayDetected,
		Agent:     agent,
		Severity:  "critical",
		Timestamp: time.Now().UTC(),
		Message: fmt.Sprintf(
			"Agent '%s' exceeded runaway threshold (%d requests in %ds)",
			agent,
			requestCount,
			windowSeconds,
		),
		Data: map[string]any{
			"request_count":  requestCount,
			"window_seconds": windowSeconds,
		},
	}
}

// NewErrorSpikeAlert creates an error-spike alert event.
func NewErrorSpikeAlert(agent string, errorRatePct float64, windowSeconds int) Alert {
	return Alert{
		Type:      TypeErrorSpike,
		Agent:     agent,
		Severity:  "warning",
		Timestamp: time.Now().UTC(),
		Message: fmt.Sprintf(
			"Agent '%s' error spike detected (%.2f%% over %ds)",
			agent,
			errorRatePct,
			windowSeconds,
		),
		Data: map[string]any{
			"error_rate_pct": errorRatePct,
			"window_seconds": windowSeconds,
		},
	}
}

// NewAgentKilledAlert creates an agent-killed alert event.
func NewAgentKilledAlert(agent string, reason string) Alert {
	return Alert{
		Type:      TypeAgentKilled,
		Agent:     agent,
		Severity:  "critical",
		Action:    "kill",
		Timestamp: time.Now().UTC(),
		Message:   fmt.Sprintf("Agent '%s' was killed (%s)", agent, reason),
		Data: map[string]any{
			"reason": reason,
		},
	}
}
