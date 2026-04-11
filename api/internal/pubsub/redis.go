package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/redis/go-redis/v9"
)

// PubSub wraps Redis pub/sub for agent directive delivery and scan events.
type PubSub struct {
	client *redis.Client
}

func New(redisURL string) (*PubSub, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parsing redis URL: %w", err)
	}

	client := redis.NewClient(opts)

	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("pinging redis: %w", err)
	}

	return &PubSub{client: client}, nil
}

func (ps *PubSub) Close() error {
	return ps.client.Close()
}

func (ps *PubSub) Ping(ctx context.Context) error {
	return ps.client.Ping(ctx).Err()
}

// Directive is a scan directive sent to an agent.
type Directive struct {
	ScanID        string `json:"scan_id"`
	BundleID      string `json:"bundle_id"`
	BundleVersion string `json:"bundle_version,omitempty"`
	TargetID      string `json:"target_id"`
}

// PublishDirective sends a scan directive to a specific agent's channel.
func (ps *PubSub) PublishDirective(ctx context.Context, agentID string, directive Directive) error {
	channel := fmt.Sprintf("agent:%s:directives", agentID)
	data, err := json.Marshal(directive)
	if err != nil {
		return fmt.Errorf("marshaling directive: %w", err)
	}

	if err := ps.client.Publish(ctx, channel, data).Err(); err != nil {
		return fmt.Errorf("publishing directive: %w", err)
	}

	slog.Info("published directive", "agent_id", agentID, "scan_id", directive.ScanID)
	return nil
}

// SubscribeDirectives subscribes to directives for a specific agent.
// The callback is invoked for each directive received.
func (ps *PubSub) SubscribeDirectives(ctx context.Context, agentID string, callback func(Directive)) error {
	channel := fmt.Sprintf("agent:%s:directives", agentID)
	sub := ps.client.Subscribe(ctx, channel)
	defer sub.Close()

	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				return nil
			}

			var directive Directive
			if err := json.Unmarshal([]byte(msg.Payload), &directive); err != nil {
				slog.Warn("invalid directive payload", "error", err)
				continue
			}

			callback(directive)
		}
	}
}

// PublishScanProgress publishes a scan status update for real-time UI.
func (ps *PubSub) PublishScanProgress(ctx context.Context, scanID string, status string) error {
	channel := fmt.Sprintf("scan:%s:progress", scanID)
	data, _ := json.Marshal(map[string]string{"scan_id": scanID, "status": status})
	return ps.client.Publish(ctx, channel, data).Err()
}
