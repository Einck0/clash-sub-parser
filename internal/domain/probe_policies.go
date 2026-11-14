package domain

import "context"

// ProbePolicySubscriptionReader supplies the subscription fields used by probe
// scheduling (ID, Enabled and Config), without display metadata or node counts.
// Repositories can implement this alongside SubscriptionRepository.
type ProbePolicySubscriptionReader interface {
	ListProbePolicies(ctx context.Context) ([]Subscription, error)
}
