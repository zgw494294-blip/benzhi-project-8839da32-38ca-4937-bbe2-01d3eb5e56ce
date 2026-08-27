package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"stage-rigging-safety-release/internal/domain"
	"stage-rigging-safety-release/internal/inspection"
	"stage-rigging-safety-release/internal/storage"
)

type Service struct {
	repo        storage.Repository
	now         func() time.Time
	permitMu    sync.Mutex
	permitCache map[string]permitLookup
}

func New(repo storage.Repository) *Service {
	return &Service{repo: repo, now: func() time.Time { return time.Now().UTC() }, permitCache: map[string]permitLookup{}}
}

type permitLookup struct {
	campaign *domain.InspectionCampaign
	err      error
}

func (s *Service) findPermit(ctx context.Context, number string) (*domain.InspectionCampaign, error) {
	s.permitMu.Lock()
	if cached, ok := s.permitCache[number]; ok {
		s.permitMu.Unlock()
		return cached.campaign, cached.err
	}
	s.permitMu.Unlock()

	campaign, err := s.repo.FindPermit(ctx, number)
	s.permitMu.Lock()
	s.permitCache[number] = permitLookup{campaign: campaign, err: err}
	s.permitMu.Unlock()
	return campaign, err
}

type CampaignView struct {
	Campaign    *domain.InspectionCampaign     `json:"campaign"`
	Coverage    inspection.CoverageReport      `json:"coverage"`
	Readiness   inspection.Readiness           `json:"readiness"`
	NextActions []string                       `json:"nextActions"`
	Instruments []inspection.InstrumentSummary `json:"instruments"`
}
type Verification struct {
	Valid              bool                    `json:"valid"`
	Permit             *domain.OperatingPermit `json:"permit,omitempty"`
	RecalculatedDigest string                  `json:"recalculatedDigest,omitempty"`
	Message            string                  `json:"message"`
	TheatreName        string                  `json:"theatreName,omitempty"`
	InspectionYear     int                     `json:"inspectionYear,omitempty"`
	Status             string                  `json:"status,omitempty"`
	RemainingDays      int                     `json:"remainingDays"`
	AssetCode          string                  `json:"assetCode,omitempty"`
	AssetInScope       *bool                   `json:"assetInScope,omitempty"`
	CheckedAt          time.Time               `json:"checkedAt"`
	Checks             map[string]bool         `json:"checks,omitempty"`
	Failures           []string                `json:"failures,omitempty"`
}

func identifier(prefix string) string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return prefix + "-" + hex.EncodeToString(b[:])
}
func operationSignature(operation string, value any) string {
	b, _ := json.Marshal(value)
	sum := sha256.Sum256(b)
	return operation + "|" + hex.EncodeToString(sum[:])
}
func auditOperation(operation string) string {
	if at := strings.IndexByte(operation, '|'); at >= 0 {
		return operation[:at]
	}
	return operation
}
func requireRole(got, want string) error {
	if got != want {
		return fmt.Errorf("%w: 需要 %s 角色", domain.ErrForbidden, want)
	}
	return nil
}

func (s *Service) CreateCampaign(ctx context.Context, cmd CreateCampaignCommand) (*domain.InspectionCampaign, error) {
	if err := requireRole(cmd.Role, "inspector"); err != nil {
		return nil, err
	}
	if cmd.ID == "" {
		cmd.ID = identifier("CAM")
	}
	c, err := domain.NewCampaign(cmd.ID, cmd.TheatreName, cmd.InspectionYear, cmd.LeadInspector, s.now())
	if err != nil {
		return nil, err
	}
	if err = s.repo.Create(ctx, c, cmd.IdempotencyKey, cmd.Actor, cmd.Role); err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, c.ID)
}

func (s *Service) GetCampaign(ctx context.Context, id string) (CampaignView, error) {
	c, err := s.repo.Get(ctx, id)
	if err != nil {
		return CampaignView{}, err
	}
	return s.makeView(c), nil
}
func (s *Service) ListCampaigns(ctx context.Context) ([]domain.InspectionCampaign, error) {
	return s.repo.List(ctx)
}
func (s *Service) Timeline(ctx context.Context, id string) ([]domain.AuditEvent, error) {
	return s.repo.Timeline(ctx, id)
}

func (s *Service) makeView(c *domain.InspectionCampaign) CampaignView {
	v := CampaignView{Campaign: c, Coverage: inspection.Coverage(*c), Readiness: inspection.ReviewReadinessAt(*c, s.now()), NextActions: []string{}, Instruments: inspection.InstrumentSummaries(*c, s.now())}
	switch c.Status {
	case domain.StatusDraft:
		v.NextActions = []string{"登记设备", "确认方案"}
	case domain.StatusExecuting:
		v.NextActions = []string{"提交实测", "整改复验", "送交复核"}
	case domain.StatusReviewPending:
		v.NextActions = []string{"复核退回或批准"}
	case domain.StatusApproved:
		v.NextActions = []string{"冻结证据包"}
	case domain.StatusFrozen:
		v.NextActions = []string{"签发启用许可"}
	case domain.StatusLicensed:
		v.NextActions = []string{"验真许可", "查看审计时间线"}
	}
	return v
}

func (s *Service) mutate(ctx context.Context, id string, m Metadata, op string, fn storage.Mutation) (*domain.InspectionCampaign, error) {
	c, _, err := s.repo.Mutate(ctx, id, m.ExpectedVersion, m.IdempotencyKey, op, m.Actor, m.Role, fn)
	if err != nil {
		_ = s.repo.AppendDecision(ctx, id, auditOperation(op), m.Actor, m.Role, false, err.Error(), m.ExpectedVersion)
	}
	return c, err
}
