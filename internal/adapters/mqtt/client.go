// Package mqtt adapts the Paho MQTT client to the application ports: it
// publishes readings, status reports and alerts, and decodes subscriptions.
package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
)

type Options struct {
	Broker   string
	ClientID string
	Username string
	Password string
	// Handlers are (re)subscribed on every successful connection, so a broker
	// restart never leaves a service silently deaf.
	Handlers map[string]paho.MessageHandler
}

// Client is a connected MQTT session with automatic reconnection.
type Client struct {
	c         paho.Client
	log       *slog.Logger
	connected atomic.Bool
	timeout   time.Duration
}

// Dial starts connecting in the background and returns immediately: an edge
// agent must keep polling (and buffering) even if the broker is down at boot.
func Dial(log *slog.Logger, o Options) *Client {
	cl := &Client{log: log, timeout: 5 * time.Second}
	opts := paho.NewClientOptions().
		AddBroker(o.Broker).
		SetClientID(o.ClientID).
		SetUsername(o.Username).
		SetPassword(o.Password).
		SetCleanSession(true).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(2 * time.Second).
		// Capped low on purpose: the edge agent buffers while offline, and
		// every extra second of backoff delays the replay.
		SetMaxReconnectInterval(10 * time.Second).
		SetOrderMatters(false)

	opts.SetOnConnectHandler(func(c paho.Client) {
		log.Info("mqtt connected", "broker", o.Broker)
		for topic, h := range o.Handlers {
			if tok := c.Subscribe(topic, 1, h); tok.Wait() && tok.Error() != nil {
				log.Error("mqtt subscribe failed", "topic", topic, "err", tok.Error())
			}
		}
		cl.connected.Store(true)
	})
	opts.SetConnectionLostHandler(func(_ paho.Client, err error) {
		cl.connected.Store(false)
		log.Warn("mqtt connection lost", "err", err)
	})

	cl.c = paho.NewClient(opts)
	cl.c.Connect()
	return cl
}

// WaitConnected blocks until the first connection succeeds or ctx ends.
func (cl *Client) WaitConnected(ctx context.Context) error {
	t := time.NewTicker(200 * time.Millisecond)
	defer t.Stop()
	for !cl.connected.Load() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
	return nil
}

func (cl *Client) Connected() bool { return cl.connected.Load() }

// PublishJSON sends v with QoS 1 and waits for the broker acknowledgement.
func (cl *Client) PublishJSON(ctx context.Context, topic string, v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tok := cl.c.Publish(topic, 1, false, payload)
	select {
	case <-tok.Done():
		return tok.Error()
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(cl.timeout):
		return fmt.Errorf("mqtt publish %s: timeout", topic)
	}
}

func (cl *Client) Close() {
	cl.c.Disconnect(250)
}

// JSONHandler decodes each message into T and passes it with its topic to fn.
// Malformed payloads are logged and skipped instead of crashing the consumer.
func JSONHandler[T any](log *slog.Logger, fn func(topic string, v T)) paho.MessageHandler {
	return func(_ paho.Client, m paho.Message) {
		var v T
		if err := json.Unmarshal(m.Payload(), &v); err != nil {
			log.Warn("dropping malformed message", "topic", m.Topic(), "err", err)
			return
		}
		fn(m.Topic(), v)
	}
}
