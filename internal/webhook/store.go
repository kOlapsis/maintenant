// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package webhook

import "context"

// WebhookSubscriptionStore defines the persistence interface for webhook subscriptions.
type WebhookSubscriptionStore interface {
	List(ctx context.Context) ([]*WebhookSubscription, error)
	GetByID(ctx context.Context, id string) (*WebhookSubscription, error)
	Create(ctx context.Context, sub *WebhookSubscription) error
	Delete(ctx context.Context, id string) error
	// RecordDelivery stores a delivery outcome: a failure counts toward the automatic disable, a success resets the count and re-enables.
	RecordDelivery(ctx context.Context, id string, delivered bool) error
	ListActive(ctx context.Context) ([]*WebhookSubscription, error)
}
