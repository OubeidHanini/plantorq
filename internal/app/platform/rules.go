package platform

import (
	"context"
	"fmt"

	"github.com/OubeidHanini/plantorq/internal/domain/alerting"
	"github.com/OubeidHanini/plantorq/internal/domain/identity"
)

type RuleService struct {
	rules    RuleRepository
	machines MachineRepository
}

func NewRuleService(rules RuleRepository, machines MachineRepository) *RuleService {
	return &RuleService{rules: rules, machines: machines}
}

func (s *RuleService) List(ctx context.Context, p identity.Principal) ([]alerting.Rule, error) {
	return s.rules.ListRules(ctx, p.TenantID)
}

// Create validates and stores a rule. Only admins may change alerting
// configuration; tenant ownership is taken from the principal, never from
// the request body.
func (s *RuleService) Create(ctx context.Context, p identity.Principal, r alerting.Rule) (alerting.Rule, error) {
	if err := s.prepare(ctx, p, &r); err != nil {
		return alerting.Rule{}, err
	}
	return s.rules.CreateRule(ctx, r)
}

func (s *RuleService) Update(ctx context.Context, p identity.Principal, r alerting.Rule) (alerting.Rule, error) {
	if err := s.prepare(ctx, p, &r); err != nil {
		return alerting.Rule{}, err
	}
	return s.rules.UpdateRule(ctx, r)
}

func (s *RuleService) Delete(ctx context.Context, p identity.Principal, id int64) error {
	if err := require(p, identity.RoleAdmin); err != nil {
		return err
	}
	return s.rules.DeleteRule(ctx, p.TenantID, id)
}

func (s *RuleService) prepare(ctx context.Context, p identity.Principal, r *alerting.Rule) error {
	if err := require(p, identity.RoleAdmin); err != nil {
		return err
	}
	r.TenantID, r.Tenant = p.TenantID, p.TenantSlug
	if err := r.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if r.Machine == "" {
		return nil
	}
	ok, err := s.machines.MachineExists(ctx, p.TenantID, r.Machine)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: unknown machine %q", ErrInvalid, r.Machine)
	}
	return nil
}
