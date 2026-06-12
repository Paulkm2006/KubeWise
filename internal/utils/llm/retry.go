package llm

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/kubewise/kubewise/internal/config"
	"go.uber.org/zap"
)

type RetryPolicy struct {
	MaxAttempts int
	Backoff     []time.Duration
}

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 3,
		Backoff:     []time.Duration{300 * time.Millisecond, time.Second, 3 * time.Second},
	}
}

func IsTransientError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return errors.Is(err, context.DeadlineExceeded)
	}
	msg := strings.ToLower(err.Error())
	for _, needle := range []string{
		"timeout",
		"deadline exceeded",
		"429",
		"too many requests",
		"rate limit",
		"502",
		"503",
		"504",
		"connection reset",
		"connection refused",
		"temporary failure",
		"temporarily unavailable",
		"service unavailable",
		"bad gateway",
		"gateway timeout",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

func CompleteWithRetry(ctx context.Context, client ClientPort, req CompletionRequest, policy RetryPolicy) (*CompletionResponse, error) {
	if client == nil {
		return nil, errors.New("llm client unavailable")
	}
	if policy.MaxAttempts <= 0 {
		policy = DefaultRetryPolicy()
	}
	var lastErr error
	for attempt := 0; attempt < policy.MaxAttempts; attempt++ {
		resp, err := client.Complete(ctx, req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !IsTransientError(err) || attempt == policy.MaxAttempts-1 {
			return nil, err
		}
		config.L().Warn("llm transient failure, retrying",
			zap.Int("attempt", attempt+1),
			zap.Int("max_attempts", policy.MaxAttempts),
			zap.Duration("backoff", policy.backoffFor(attempt)),
			zap.Error(err),
		)
		if err := sleepWithContext(ctx, policy.backoffFor(attempt)); err != nil {
			return nil, err
		}
	}
	return nil, lastErr
}

func (p RetryPolicy) backoffFor(attempt int) time.Duration {
	if len(p.Backoff) == 0 {
		return time.Duration(attempt+1) * 300 * time.Millisecond
	}
	if attempt >= len(p.Backoff) {
		return p.Backoff[len(p.Backoff)-1]
	}
	return p.Backoff[attempt]
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
