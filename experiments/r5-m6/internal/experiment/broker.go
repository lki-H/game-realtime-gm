package experiment

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Broker struct {
	Connection *amqp.Connection
	Channel    *amqp.Channel
	returns    chan amqp.Return
	mutex      sync.Mutex
}

func OpenBroker(cfg BrokerConfig) (*Broker, error) {
	endpoint := url.URL{Scheme: "amqp", Host: cfg.Address, Path: "/" + cfg.VHost, User: url.UserPassword(cfg.User, cfg.Password)}
	connection, err := amqp.DialConfig(endpoint.String(), amqp.Config{Dial: amqp.DefaultDial(3 * time.Second), Heartbeat: 10 * time.Second})
	if err != nil {
		return nil, errors.New("broker_connection_failed")
	}
	channel, err := connection.Channel()
	if err != nil {
		connection.Close()
		return nil, errors.New("broker_channel_failed")
	}
	if err := channel.Confirm(false); err != nil {
		channel.Close()
		connection.Close()
		return nil, errors.New("broker_confirm_unavailable")
	}
	returned := channel.NotifyReturn(make(chan amqp.Return, 1))
	return &Broker{Connection: connection, Channel: channel, returns: returned}, nil
}
func (broker *Broker) Close() { broker.Channel.Close(); broker.Connection.Close() }
func (broker *Broker) Publish(ctx context.Context, destination string, data []byte, headers map[string]any) error {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	if destination != "events" && destination != "retry" && destination != "dead" {
		return errors.New("invalid_broker_destination")
	}
	var envelope Envelope
	_ = json.Unmarshal(data, &envelope)
	confirmation, err := broker.Channel.PublishWithDeferredConfirmWithContext(ctx, "r5."+destination, "settled", true, false, amqp.Publishing{ContentType: "application/json", DeliveryMode: amqp.Persistent, MessageId: envelope.MessageID, Headers: amqp.Table(headers), Body: data})
	if err != nil {
		return errors.New("broker_publish_failed")
	}
	if confirmation == nil {
		return errors.New("broker_confirm_unavailable")
	}
	confirmed, err := confirmation.WaitContext(ctx)
	if err != nil || !confirmed {
		return errors.New("broker_confirm_failed")
	}
	select {
	case <-broker.returns:
		return errors.New("broker_message_unroutable")
	default:
	}
	return nil
}
func DeclareTopology(channel *amqp.Channel, retryDelayMS int) error {
	for _, exchange := range []string{"r5.events", "r5.retry", "r5.dead"} {
		if err := channel.ExchangeDeclare(exchange, "direct", true, false, false, false, nil); err != nil {
			return err
		}
	}
	for _, definition := range []struct {
		name, exchange string
		args           amqp.Table
	}{
		{"r5.reports", "r5.events", amqp.Table{"x-queue-type": "quorum", "x-max-length": int32(10000), "x-overflow": "reject-publish", "x-dead-letter-strategy": "at-least-once", "x-dead-letter-exchange": "r5.dead", "x-dead-letter-routing-key": "settled"}},
		{"r5.retry.queue", "r5.retry", amqp.Table{"x-queue-type": "quorum", "x-message-ttl": int32(retryDelayMS), "x-dead-letter-strategy": "at-least-once", "x-dead-letter-exchange": "r5.events", "x-dead-letter-routing-key": "settled", "x-max-length": int32(10000), "x-overflow": "reject-publish"}},
		{"r5.dead.queue", "r5.dead", amqp.Table{"x-queue-type": "quorum", "x-max-length": int32(10000), "x-overflow": "reject-publish"}},
	} {
		if _, err := channel.QueueDeclare(definition.name, true, false, false, false, definition.args); err != nil {
			return err
		}
		if err := channel.QueueBind(definition.name, "settled", definition.exchange, false, nil); err != nil {
			return err
		}
	}
	return nil
}
