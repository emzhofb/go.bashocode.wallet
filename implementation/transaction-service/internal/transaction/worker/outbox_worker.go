package worker

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/emzhofb/gowallet/pkg/rabbitmq"
	"github.com/emzhofb/gowallet/transaction-service/internal/transaction/repository"
)

type OutboxWorker struct {
	repo *repository.TransactionRepository
	rmq  *rabbitmq.RabbitMQ
}

func NewOutboxWorker(repo repository.TransactionRepository, rmq *rabbitmq.RabbitMQ) *OutboxWorker {
	return &OutboxWorker{
		repo: &repo,
		rmq:  rmq,
	}
}

func (w *OutboxWorker) Start(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.processEvents(ctx)
		}
	}
}

func (w *OutboxWorker) processEvents(ctx context.Context) {
	events, err := (*w.repo).GetPendingOutbox(ctx)
	if err != nil {
		log.Printf("Outbox worker error getting pending events: %v", err)
		return
	}

	for _, event := range events {
		var payloadMap map[string]interface{}
		err := json.Unmarshal([]byte(event.Payload), &payloadMap)
		if err != nil {
			log.Printf("Outbox worker failed to unmarshal payload for event %s: %v", event.ID, err)
			continue
		}

		// Publish to RabbitMQ exchange
		err = w.rmq.Publish(ctx, "gowallet.events", event.EventType, payloadMap)
		if err != nil {
			log.Printf("Outbox worker failed to publish event %s: %v", event.ID, err)
			continue
		}

		err = (*w.repo).MarkOutboxProcessed(ctx, event.ID)
		if err != nil {
			log.Printf("Outbox worker failed to mark event %s as processed: %v", event.ID, err)
		} else {
			log.Printf("Outbox worker successfully published event %s: %s", event.ID, event.EventType)
		}
	}
}
