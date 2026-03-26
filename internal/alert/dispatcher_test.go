package alert

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OberWatch/oberwatch/internal/config"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func newTestClient(fn roundTripFunc) *http.Client {
	return &http.Client{Transport: fn}
}

func responseWithStatus(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestAlertDispatcher_WebhookSendsJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "webhook receives alert json payload"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			captured := make(chan []byte, 1)
			client := newTestClient(func(request *http.Request) (*http.Response, error) {
				body, err := io.ReadAll(request.Body)
				if err != nil {
					t.Fatalf("ReadAll() error = %v", err)
				}
				captured <- body
				return responseWithStatus(http.StatusOK, "ok"), nil
			})

			dispatcher := NewDispatcherWithClient(config.AlertsConfig{WebhookURL: "https://alerts.example/webhook"}, time.Second, nil, client)
			event := Alert{
				Type:            TypeBudgetThreshold,
				Agent:           "email-agent",
				ThresholdPct:    80,
				SpentUSD:        8,
				LimitUSD:        10,
				Action:          "downgrade",
				Message:         "threshold reached",
				Severity:        "warning",
				Timestamp:       time.Now().UTC(),
				PeriodStartedAt: time.Date(2026, time.March, 26, 0, 0, 0, 0, time.UTC),
			}

			dispatcher.Dispatch(context.Background(), event)

			select {
			case payload := <-captured:
				var decoded Alert
				if err := json.Unmarshal(payload, &decoded); err != nil {
					t.Fatalf("Unmarshal() error = %v", err)
				}
				if decoded.Type != TypeBudgetThreshold {
					t.Fatalf("type = %q, want %q", decoded.Type, TypeBudgetThreshold)
				}
				if decoded.Agent != "email-agent" {
					t.Fatalf("agent = %q, want %q", decoded.Agent, "email-agent")
				}
			default:
				t.Fatal("no webhook payload captured")
			}
		})
	}
}

func TestAlertDispatcher_SlackFormatsMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "slack payload contains formatted fields"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			captured := make(chan []byte, 1)
			client := newTestClient(func(request *http.Request) (*http.Response, error) {
				body, err := io.ReadAll(request.Body)
				if err != nil {
					t.Fatalf("ReadAll() error = %v", err)
				}
				captured <- body
				return responseWithStatus(http.StatusOK, "ok"), nil
			})

			dispatcher := NewDispatcherWithClient(config.AlertsConfig{SlackWebhookURL: "https://hooks.slack.com/services/abc"}, time.Second, nil, client)
			event := Alert{
				Type:         TypeBudgetThreshold,
				Agent:        "finance-agent",
				ThresholdPct: 50,
				SpentUSD:     5,
				LimitUSD:     10,
				Action:       "alert",
				Message:      "Budget threshold reached",
				Severity:     "warning",
				Timestamp:    time.Now().UTC(),
			}

			dispatcher.Dispatch(context.Background(), event)

			select {
			case payload := <-captured:
				var decoded map[string]any
				if err := json.Unmarshal(payload, &decoded); err != nil {
					t.Fatalf("Unmarshal() error = %v", err)
				}
				text, ok := decoded["text"].(string)
				if !ok || !strings.Contains(text, string(TypeBudgetThreshold)) {
					t.Fatalf("text = %#v, want alert type", decoded["text"])
				}
				blocks, ok := decoded["blocks"].([]any)
				if !ok || len(blocks) == 0 {
					t.Fatalf("blocks = %#v, want non-empty array", decoded["blocks"])
				}
			default:
				t.Fatal("no slack payload captured")
			}
		})
	}
}

func TestAlertDispatcher_DeduplicatesThresholdsPerPeriod(t *testing.T) {
	t.Parallel()

	//nolint:govet // keep test table fields explicit.
	tests := []struct {
		name            string
		secondThreshold float64
		wantCalls       int32
	}{
		{name: "same threshold same period deduped", secondThreshold: 80, wantCalls: 1},
		{name: "different threshold same period not deduped", secondThreshold: 100, wantCalls: 2},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var calls int32
			client := newTestClient(func(request *http.Request) (*http.Response, error) {
				atomic.AddInt32(&calls, 1)
				return responseWithStatus(http.StatusOK, "ok"), nil
			})
			dispatcher := NewDispatcherWithClient(config.AlertsConfig{WebhookURL: "https://alerts.example/webhook"}, time.Second, nil, client)

			periodStart := time.Date(2026, time.March, 26, 0, 0, 0, 0, time.UTC)
			base := Alert{
				Type:            TypeBudgetThreshold,
				Agent:           "email-agent",
				ThresholdPct:    80,
				SpentUSD:        8,
				LimitUSD:        10,
				Action:          "downgrade",
				Message:         "threshold reached",
				Severity:        "warning",
				Timestamp:       time.Now().UTC(),
				PeriodStartedAt: periodStart,
			}
			second := base
			second.ThresholdPct = tt.secondThreshold

			dispatcher.Dispatch(context.Background(), base)
			dispatcher.Dispatch(context.Background(), second)

			if got := atomic.LoadInt32(&calls); got != tt.wantCalls {
				t.Fatalf("dispatch calls = %d, want %d", got, tt.wantCalls)
			}
		})
	}
}

func TestAlertConstructors_AllTypes(t *testing.T) {
	t.Parallel()

	//nolint:govet // keep constructor function field explicit.
	tests := []struct {
		name     string
		wantType Type
		build    func() Alert
	}{
		{
			name:     "budget threshold",
			wantType: TypeBudgetThreshold,
			build: func() Alert {
				return NewBudgetThresholdAlert("agent-a", 80, 8, 10, "downgrade", time.Now().UTC())
			},
		},
		{
			name:     "budget exceeded",
			wantType: TypeBudgetExceeded,
			build: func() Alert {
				return NewBudgetExceededAlert("agent-a", 11, 10, "reject")
			},
		},
		{
			name:     "runaway detected",
			wantType: TypeRunawayDetected,
			build: func() Alert {
				return NewRunawayDetectedAlert("agent-a", 120, 60)
			},
		},
		{
			name:     "error spike",
			wantType: TypeErrorSpike,
			build: func() Alert {
				return NewErrorSpikeAlert("agent-a", 42.5, 60)
			},
		},
		{
			name:     "agent killed",
			wantType: TypeAgentKilled,
			build: func() Alert {
				return NewAgentKilledAlert("agent-a", "runaway")
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.build()
			if got.Type != tt.wantType {
				t.Fatalf("type = %q, want %q", got.Type, tt.wantType)
			}
			if got.Agent == "" {
				t.Fatal("agent should not be empty")
			}
			if got.Message == "" {
				t.Fatal("message should not be empty")
			}
		})
	}
}

func TestAlertDispatcher_WebhookFailureHandledGracefully(t *testing.T) {
	t.Parallel()

	//nolint:govet // keep test table fields explicit.
	tests := []struct {
		name      string
		transport roundTripFunc
		wantCalls int32
	}{
		{
			name: "failing webhook retries once",
			transport: func(request *http.Request) (*http.Response, error) {
				return responseWithStatus(http.StatusInternalServerError, "fail"), nil
			},
			wantCalls: 2,
		},
		{
			name: "network error retries once",
			transport: func(request *http.Request) (*http.Response, error) {
				return nil, errors.New("network down")
			},
			wantCalls: 2,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var calls int32
			client := newTestClient(func(request *http.Request) (*http.Response, error) {
				atomic.AddInt32(&calls, 1)
				return tt.transport(request)
			})
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			dispatcher := NewDispatcherWithClient(config.AlertsConfig{WebhookURL: "https://alerts.example/webhook"}, time.Second, logger, client)
			dispatcher.Dispatch(context.Background(), NewBudgetExceededAlert("agent-a", 11, 10, "reject"))

			if got := atomic.LoadInt32(&calls); got != tt.wantCalls {
				t.Fatalf("webhook calls = %d, want %d", got, tt.wantCalls)
			}
		})
	}
}

func TestAlertDispatcher_NilAndNoDestinations(t *testing.T) {
	t.Parallel()

	//nolint:govet // keep test table fields explicit.
	tests := []struct {
		name            string
		dispatcher      *AlertDispatcher
		expectTransport bool
	}{
		{name: "nil dispatcher is safe", dispatcher: nil, expectTransport: false},
		{name: "empty destinations does not send", dispatcher: NewDispatcherWithClient(config.AlertsConfig{}, time.Second, nil, newTestClient(func(request *http.Request) (*http.Response, error) {
			t.Fatal("unexpected transport call")
			return responseWithStatus(http.StatusOK, "ok"), nil
		})), expectTransport: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.dispatcher == nil {
				var nilDispatcher *AlertDispatcher
				nilDispatcher.Dispatch(context.Background(), Alert{Type: TypeBudgetExceeded, Agent: "a", Timestamp: time.Now().UTC()})
				return
			}
			tt.dispatcher.Dispatch(context.Background(), Alert{Type: TypeBudgetExceeded, Agent: "a", Timestamp: time.Now().UTC()})
		})
	}
}
