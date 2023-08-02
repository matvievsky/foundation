package foundation

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/ri-nat/foundation/outboxrepo"
	"google.golang.org/protobuf/proto"

	fctx "github.com/ri-nat/foundation/context"
	fkafka "github.com/ri-nat/foundation/kafka"
)

var (
	OutboxEnabled = GetEnvOrBool("OUTBOX_ENABLED", false)
)

// Event represents an event to be published to the outbox
type Event struct {
	Topic     string
	Key       string
	Payload   []byte
	ProtoName string
	Headers   map[string]string
	CreatedAt time.Time
}

// Unmarshal unmarshals the event payload into a protobuf message
func (e *Event) Unmarshal(msg proto.Message) FoundationError {
	if err := proto.Unmarshal(e.Payload, msg); err != nil {
		return NewInternalError(err, "failed to unmarshal Event payload")
	}

	return nil
}

// NewEventFromProto creates a new event from a protobuf message
func NewEventFromProto(msg proto.Message, key string, headers map[string]string) (*Event, FoundationError) {
	payload, err := proto.Marshal(msg)
	if err != nil {
		return nil, NewInternalError(err, "failed to marshal message")
	}

	// Get proto name
	protoName := string(msg.ProtoReflect().Descriptor().FullName())
	// Construct topic name from proto name
	topic := protoNameToTopic(protoName)

	if headers == nil {
		headers = make(map[string]string)
	}

	return &Event{
		Topic:     topic,
		Key:       key,
		Payload:   payload,
		ProtoName: protoName,
		Headers:   headers,
		CreatedAt: time.Now(),
	}, nil
}

func addDefaultHeaders(ctx context.Context, event *Event) *Event {
	if event.Headers == nil {
		event.Headers = make(map[string]string)
	}

	event.Headers[fkafka.HeaderProtoName] = event.ProtoName
	event.Headers[fkafka.HeaderCorrelationID] = fctx.GetCorrelationID(ctx)

	return event
}

// publishEventToOutbox publishes an event to the outbox.
func (app *Application) publishEventToOutbox(ctx context.Context, event *Event, tx *sql.Tx) FoundationError {
	commitNeeded := false

	if tx == nil {
		// Start transaction
		tx, err := app.GetPostgreSQL().Begin()
		if err != nil {
			return NewInternalError(err, "failed to begin transaction")
		}
		defer tx.Rollback() // nolint:errcheck
		commitNeeded = true
	}

	// Marshal headers to JSON
	headers, err := json.Marshal(event.Headers)
	if err != nil {
		return NewInternalError(err, "failed to marshal headers")
	}

	queries := outboxrepo.New(tx)
	params := outboxrepo.CreateOutboxEventParams{
		Topic:   event.Topic,
		Key:     event.Key,
		Payload: event.Payload,
		Headers: headers,
	}
	// Publish event
	if err := queries.CreateOutboxEvent(ctx, params); err != nil {
		return NewInternalError(err, "failed to insert event into outbox")
	}

	if commitNeeded {
		if err = tx.Commit(); err != nil {
			return NewInternalError(err, "failed to commit transaction")
		}
	}

	return nil
}

// publishEventToKafka publishes an event to the Kafka topic.
func (app *Application) publishEventToKafka(ctx context.Context, event *Event) FoundationError {
	message, err := NewMessageFromEvent(event)
	if err != nil {
		return NewInternalError(err, "failed to create message from event")
	}
	ch := make(chan kafka.Event)
	if err := app.GetKafkaProducer().Produce(message, ch); err != nil {
		return NewInternalError(err, "failed to publish event to Kafka")
	}

	// Wait for delivery report
	e := <-ch
	m := e.(*kafka.Message)
	if m.TopicPartition.Error != nil {
		return NewInternalError(m.TopicPartition.Error, "failed to publish event to Kafka")
	}

	return nil
}

// PublishEvent publishes an event to the outbox, starting a new transaction,
// or straight to the Kafka topic if `OUTBOX_ENABLED` is not set.
func (app *Application) PublishEvent(ctx context.Context, event *Event, tx *sql.Tx) FoundationError {
	event = addDefaultHeaders(ctx, event)

	if OutboxEnabled {
		return app.publishEventToOutbox(ctx, event, tx)
	}

	return app.publishEventToKafka(ctx, event)
}

// NewAndPublishEvent creates a new event and publishes it to the outbox within a transaction
func (app *Application) NewAndPublishEvent(ctx context.Context, msg proto.Message, key string, headers map[string]string, tx *sql.Tx) FoundationError {
	event, err := NewEventFromProto(msg, key, headers)
	if err != nil {
		return err
	}

	return app.PublishEvent(ctx, event, tx)
}

// WithTransaction executes the given function in a transaction. If the function
// returns an event, it will be published.
func (app *Application) WithTransaction(ctx context.Context, f func(tx *sql.Tx) (*Event, FoundationError)) FoundationError {
	// Start transaction
	tx, err := app.GetPostgreSQL().Begin()
	if err != nil {
		return NewInternalError(err, "failed to begin transaction")
	}
	defer tx.Rollback() // nolint: errcheck

	// Execute function
	event, ferr := f(tx)
	if ferr != nil {
		return ferr
	}

	// Publish event (if any)
	if event != nil {
		if err = app.PublishEvent(ctx, event, tx); err != nil {
			return NewInternalError(err, "failed to publish event")
		}
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		return NewInternalError(err, "failed to commit transaction")
	}

	return nil
}

// ListOutboxEvents returns a list of outbox events in the order they were created.
func (app *Application) ListOutboxEvents(ctx context.Context, limit int32) ([]outboxrepo.FoundationOutboxEvent, FoundationError) {
	queries := outboxrepo.New(app.GetPostgreSQL())

	events, err := queries.ListOutboxEvents(ctx, limit)
	if err != nil {
		return nil, NewInternalError(err, "failed to `ListOutboxEvents`")
	}

	return events, nil
}

// DeleteOutboxEvents deletes outbox events up to (and including) the given ID.
func (app *Application) DeleteOutboxEvents(ctx context.Context, maxID int64) FoundationError {
	queries := outboxrepo.New(app.GetPostgreSQL())

	if err := queries.DeleteOutboxEvents(ctx, maxID); err != nil {
		return NewInternalError(err, "failed to `DeleteOutboxEvents`")
	}

	return nil
}

func protoNameToTopic(protoName string) string {
	topicParts := strings.Split(protoName, ".")
	topicParts = topicParts[:len(topicParts)-1]

	return strings.Join(topicParts, ".")
}
