// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"

	"github.com/kolapsis/maintenant/internal/alert"
)

// Mailer sends one HTML email.
type Mailer interface {
	Send(to, subject, htmlBody string) error
}

// SubscriberNotifier emails every confirmed subscriber.
type SubscriberNotifier interface {
	NotifyAll(ctx context.Context, subject, message string)
}

// AlertIncidentHandler turns alert events into status page incidents.
type AlertIncidentHandler interface {
	HandleAlertEvent(ctx context.Context, evt alert.Event)
}

// MaintenanceRunner activates and closes maintenance windows as their time comes.
type MaintenanceRunner interface {
	Start(ctx context.Context)
}

// PersonalizationReader is what the public settings.json reads.
type PersonalizationReader interface {
	GetSettings(ctx context.Context) (Settings, error)
	GetAsset(ctx context.Context, role AssetRole) (*Asset, error)
	ListFooterLinks(ctx context.Context) ([]FooterLink, error)
	ListFAQItems(ctx context.Context) ([]FAQItem, error)
}

// PersonalizationManager validates and writes the status page personalization.
type PersonalizationManager interface {
	PersonalizationReader
	UpdateSettings(ctx context.Context, in Settings) (Settings, []ContrastWarning, error)
	PutAsset(ctx context.Context, role AssetRole, mime string, data []byte, altText string) error
	DeleteAsset(ctx context.Context, role AssetRole) error
	CreateFooterLink(ctx context.Context, label, url string) (FooterLink, error)
	UpdateFooterLink(ctx context.Context, id string, label, url string) (FooterLink, error)
	DeleteFooterLink(ctx context.Context, id string) error
	ReorderFooterLinks(ctx context.Context, ids []string) ([]FooterLink, error)
	CreateFAQItem(ctx context.Context, question, answerMD string) (FAQItem, error)
	UpdateFAQItem(ctx context.Context, id string, question, answerMD string) (FAQItem, error)
	DeleteFAQItem(ctx context.Context, id string) error
	ReorderFAQItems(ctx context.Context, ids []string) ([]FAQItem, error)
	AssetSizeCap(role AssetRole) int64
	DetectAssetMIME(role AssetRole, head []byte) (string, error)
}
