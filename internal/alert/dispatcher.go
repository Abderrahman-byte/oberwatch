package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/OberWatch/oberwatch/internal/config"
)

const defaultDispatchTimeout = 5 * time.Second

// AlertDispatcher routes alerts to configured destinations.
//
//nolint:revive,govet // Name required by spec; field grouping is deliberate.
type AlertDispatcher struct {
	httpClient *http.Client
	logger     *slog.Logger
	webhookURL string
	slackURL   string
	timeout    time.Duration

	mu             sync.Mutex
	thresholdDedup map[string]struct{}
}

// NewDispatcher creates a dispatcher from alerts config.
func NewDispatcher(cfg config.AlertsConfig, timeout time.Duration, logger *slog.Logger) *AlertDispatcher {
	return NewDispatcherWithClient(cfg, timeout, logger, &http.Client{})
}

// NewDispatcherWithClient creates a dispatcher with a custom HTTP client.
func NewDispatcherWithClient(cfg config.AlertsConfig, timeout time.Duration, logger *slog.Logger, client *http.Client) *AlertDispatcher {
	if timeout <= 0 {
		timeout = defaultDispatchTimeout
	}
	if client == nil {
		client = &http.Client{}
	}

	return &AlertDispatcher{
		httpClient:     client,
		logger:         logger,
		webhookURL:     strings.TrimSpace(cfg.WebhookURL),
		slackURL:       strings.TrimSpace(cfg.SlackWebhookURL),
		timeout:        timeout,
		thresholdDedup: make(map[string]struct{}),
	}
}

// Dispatch sends the alert to configured destinations.
func (d *AlertDispatcher) Dispatch(ctx context.Context, event Alert) {
	if d == nil {
		return
	}
	if d.shouldSuppress(event) {
		return
	}

	if d.webhookURL != "" {
		if err := d.sendWebhook(ctx, event); err != nil {
			d.logWarn("webhook alert dispatch failed", err, event)
		}
	}
	if d.slackURL != "" {
		if err := d.sendSlack(ctx, event); err != nil {
			d.logWarn("slack alert dispatch failed", err, event)
		}
	}
}

func (d *AlertDispatcher) shouldSuppress(event Alert) bool {
	if event.Type != TypeBudgetThreshold {
		return false
	}
	if event.ThresholdPct <= 0 {
		return false
	}

	periodStart := event.PeriodStartedAt.UTC()
	if periodStart.IsZero() {
		periodStart = event.Timestamp.UTC()
	}
	key := fmt.Sprintf("%s|%.4f|%s", strings.ToLower(strings.TrimSpace(event.Agent)), event.ThresholdPct, periodStart.Format(time.RFC3339Nano))

	d.mu.Lock()
	defer d.mu.Unlock()
	if _, exists := d.thresholdDedup[key]; exists {
		return true
	}
	d.thresholdDedup[key] = struct{}{}
	return false
}

func (d *AlertDispatcher) sendWebhook(ctx context.Context, event Alert) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal webhook alert payload: %w", err)
	}
	return d.sendWithRetry(ctx, d.webhookURL, body)
}

func (d *AlertDispatcher) sendSlack(ctx context.Context, event Alert) error {
	payload := d.buildSlackPayload(event)
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal slack alert payload: %w", err)
	}
	return d.sendWithRetry(ctx, d.slackURL, body)
}

func (d *AlertDispatcher) sendWithRetry(ctx context.Context, url string, body []byte) error {
	var firstErr error
	for attempt := 0; attempt < 2; attempt++ {
		if err := d.sendOnce(ctx, url, body); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		return nil
	}
	return firstErr
}

func (d *AlertDispatcher) sendOnce(ctx context.Context, url string, body []byte) error {
	requestCtx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build alert request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send alert request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		responseBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("alert request returned status %d and failed reading body: %w", resp.StatusCode, readErr)
		}
		return fmt.Errorf("alert request returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	return nil
}

func (d *AlertDispatcher) buildSlackPayload(event Alert) map[string]any {
	thresholdText := "n/a"
	if event.ThresholdPct > 0 {
		thresholdText = fmt.Sprintf("%.0f%%", event.ThresholdPct)
	}
	spentLimitText := "n/a"
	if event.LimitUSD > 0 || event.SpentUSD > 0 {
		spentLimitText = fmt.Sprintf("$%.2f / $%.2f", event.SpentUSD, event.LimitUSD)
	}
	actionText := event.Action
	if actionText == "" {
		actionText = "n/a"
	}

	return map[string]any{
		"text": fmt.Sprintf("Oberwatch alert: %s", event.Type),
		"blocks": []map[string]any{
			{
				"type": "section",
				"text": map[string]any{
					"type": "mrkdwn",
					"text": fmt.Sprintf("*Oberwatch Alert* `%s`", event.Type),
				},
			},
			{
				"type": "section",
				"fields": []map[string]any{
					{"type": "mrkdwn", "text": fmt.Sprintf("*Agent:*\n%s", event.Agent)},
					{"type": "mrkdwn", "text": fmt.Sprintf("*Threshold:*\n%s", thresholdText)},
					{"type": "mrkdwn", "text": fmt.Sprintf("*Spent/Limit:*\n%s", spentLimitText)},
					{"type": "mrkdwn", "text": fmt.Sprintf("*Action:*\n%s", actionText)},
				},
			},
			{
				"type": "section",
				"text": map[string]any{
					"type": "mrkdwn",
					"text": event.Message,
				},
			},
		},
	}
}

func (d *AlertDispatcher) logWarn(message string, err error, event Alert) {
	if d.logger == nil {
		return
	}
	d.logger.Warn(message,
		"error", err,
		"type", event.Type,
		"agent", event.Agent,
	)
}
