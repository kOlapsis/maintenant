package status

import "context"

type PersonalizationStore interface {
	GetSettings(ctx context.Context) (Settings, error)
	UpdateSettings(ctx context.Context, s Settings) (Settings, error)
	BumpVersion(ctx context.Context) error

	GetAsset(ctx context.Context, role AssetRole) (*Asset, error)
	PutAsset(ctx context.Context, a Asset) error
	DeleteAsset(ctx context.Context, role AssetRole) error

	ListFooterLinks(ctx context.Context) ([]FooterLink, error)
	CreateFooterLink(ctx context.Context, label, url string) (FooterLink, error)
	UpdateFooterLink(ctx context.Context, id string, label, url string) (FooterLink, error)
	DeleteFooterLink(ctx context.Context, id string) error
	ReorderFooterLinks(ctx context.Context, ids []string) ([]FooterLink, error)

	ListFAQItems(ctx context.Context) ([]FAQItem, error)
	CreateFAQItem(ctx context.Context, question, answerMD, answerHTML string) (FAQItem, error)
	UpdateFAQItem(ctx context.Context, id string, question, answerMD, answerHTML string) (FAQItem, error)
	DeleteFAQItem(ctx context.Context, id string) error
	ReorderFAQItems(ctx context.Context, ids []string) ([]FAQItem, error)
}
