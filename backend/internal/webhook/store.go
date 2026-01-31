package webhook

import "context"

// WebhookSubscriptionStore defines the persistence interface for webhook subscriptions.
type WebhookSubscriptionStore interface {
	List(ctx context.Context) ([]*WebhookSubscription, error)
	GetByID(ctx context.Context, id string) (*WebhookSubscription, error)
	Create(ctx context.Context, sub *WebhookSubscription) error
	Delete(ctx context.Context, id string) error
	UpdateDeliveryStatus(ctx context.Context, id string, status string, failureCount int) error
	ListActive(ctx context.Context) ([]*WebhookSubscription, error)
}
