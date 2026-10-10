package experiment

import (
	"context"
	"errors"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Consumer struct {
	Projection Projection
	Sender     Sender
	SourceName string
	MaxAge     time.Duration
	RetryLimit int
	Metrics    *Metrics
	Apply      func(context.Context, Envelope, Report) (bool, error)
}

func retryNumber(headers amqp.Table, limit int) (int, bool) {
	value, present := headers["x-r5-attempt"]
	if !present {
		return 0, true
	}
	var number int64
	switch typed := value.(type) {
	case int32:
		number = int64(typed)
	case int64:
		number = typed
	default:
		return 0, false
	}
	return int(number), number >= 0 && number <= int64(limit)
}
func (consumer Consumer) Handle(ctx context.Context, delivery amqp.Delivery) error {
	attempt, valid := retryNumber(delivery.Headers, consumer.RetryLimit)
	envelope, report, err := DecodeEnvelope(delivery.Body, consumer.SourceName, time.Now().UTC(), consumer.MaxAge)
	if !valid || delivery.ContentType != "application/json" || delivery.MessageId != envelope.MessageID {
		err = ErrInvalidMessage
	}
	if errors.Is(err, ErrExpired) {
		applied, receiptErr := consumer.Projection.Applied(ctx, envelope)
		if receiptErr == nil && applied {
			consumer.Metrics.Add("duplicates", 1)
			return delivery.Ack(false)
		}
		if receiptErr != nil {
			err = receiptErr
		}
	}
	if err == nil {
		apply := consumer.Apply
		if apply == nil {
			apply = consumer.Projection.Apply
		}
		var duplicate bool
		duplicate, err = apply(ctx, envelope, report)
		if err == nil {
			if duplicate {
				consumer.Metrics.Add("duplicates", 1)
			} else {
				consumer.Metrics.Add("applied", 1)
			}
			slog.Info("r5 projection committed", "message_id", envelope.MessageID, "run_id", report.RunID, "duplicate", duplicate)
			return delivery.Ack(false)
		}
	}
	permanent := errors.Is(err, ErrInvalidMessage) || errors.Is(err, ErrExpired) || errors.Is(err, ErrConflict)
	reason := "projection_failed"
	if permanent {
		reason = err.Error()
	}
	attempt++
	dead := permanent || attempt >= consumer.RetryLimit
	if !errors.Is(err, ErrInvalidMessage) {
		storedAttempt, storedDead, failureErr := consumer.Projection.Failure(ctx, envelope, reason, attempt, consumer.RetryLimit, permanent)
		if failureErr == nil {
			attempt, dead = storedAttempt, storedDead
		}
		if errors.Is(failureErr, ErrConflict) {
			dead = true
			reason = "message_conflict"
		}
	}
	if attempt > consumer.RetryLimit {
		attempt = consumer.RetryLimit
	}
	destination := "retry"
	if dead {
		destination = "dead"
	}
	if err := consumer.Sender.Publish(ctx, destination, delivery.Body, map[string]any{"x-r5-attempt": int32(attempt), "x-r5-error": reason}); err != nil {
		_ = delivery.Nack(false, true)
		return err
	}
	consumer.Metrics.Add(destination, 1)
	slog.Warn("r5 delivery routed", "body_hash", Hash(delivery.Body), "destination", destination, "attempts", attempt, "error_code", reason)
	return delivery.Ack(false)
}
func (consumer Consumer) Run(ctx context.Context, broker *Broker) error {
	channel, err := broker.Connection.Channel()
	if err != nil {
		return errors.New("consumer_channel_failed")
	}
	defer channel.Close()
	if err := channel.Qos(10, 0, false); err != nil {
		return err
	}
	deliveries, err := channel.Consume("r5.reports", "", false, false, false, false, nil)
	if err != nil {
		return errors.New("consumer_start_failed")
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case delivery, open := <-deliveries:
			if !open {
				return errors.New("consumer_connection_closed")
			}
			iteration, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := consumer.Handle(iteration, delivery)
			cancel()
			if err != nil {
				return err
			}
		}
	}
}
