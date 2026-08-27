package storage

import (
	"context"
	"stage-rigging-safety-release/internal/domain"
)

type Mutation func(*domain.InspectionCampaign) (any, error)

type Repository interface {
	Create(context.Context, *domain.InspectionCampaign, string, string, string) error
	Get(context.Context, string) (*domain.InspectionCampaign, error)
	List(context.Context) ([]domain.InspectionCampaign, error)
	Mutate(context.Context, string, int, string, string, string, string, Mutation) (*domain.InspectionCampaign, bool, error)
	AppendDecision(context.Context, string, string, string, string, bool, string, int) error
	Timeline(context.Context, string) ([]domain.AuditEvent, error)
	FindPermit(context.Context, string) (*domain.InspectionCampaign, error)
	Close() error
}
