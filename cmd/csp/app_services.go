package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/application/iprisk"
	"clash-sub-parser/internal/application/policy"
	"clash-sub-parser/internal/application/probe"
	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/application/revision"
	"clash-sub-parser/internal/application/subscription"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
	"clash-sub-parser/internal/probe/queue"
	"clash-sub-parser/internal/repository/sqlite"
)

type applicationServices struct {
	settingsRepo        domain.SettingsRepository
	subRepo             domain.SubscriptionRepository
	nodeRepo            domain.NodeRepository
	nodeSourceRepo      domain.NodeSourceRepository
	fetchRepo           domain.SubscriptionFetchRepository
	auditRepo           domain.AuditRepository
	probeRunRepo        domain.ProbeRunRepository
	probeObsRepo        domain.ProbeObservationRepository
	policyRepo          domain.PolicyRepository
	revisionRepo        domain.RevisionRepository
	pubRepo             domain.PublicationRepository
	riskPolicyRepo      domain.RiskPolicyRevisionRepository
	riskBindingRepo     domain.RiskPolicyGroupBindingRepository
	riskObsRepo         domain.IPRiskObservationRepository
	probeScheduleRepo   domain.ProbeScheduleRepository
	nodeFilterRepo      domain.NodeFilterRepository
	pubPayloadRefRepo   domain.PublicationPayloadRefRepository

	fetchClient         *fetch.Client
	subService          *subscription.Service
	invService          *inventory.Service
	probeScheduler      *queue.Scheduler
	probeRunner         probe.Runner
	probeService        *probe.Service
	periodicCoordinator *probe.PeriodicCoordinator
	policyService       *policy.Service
	revisionService     *revision.Service
	ipriskService       *iprisk.Service
	pubService          *publication.Service
}

type appServiceOptions struct {
	fetchProxy          string
	newProbeRunner      func(db *sql.DB, nodeRepo domain.NodeRepository, obsRepo domain.ProbeObservationRepository, scheduler *queue.Scheduler, runRepo domain.ProbeRunRepository, opts ...probe.DefaultRunnerOption) probe.Runner
	probeRunnerOpts     []probe.DefaultRunnerOption
	startCoordinator    bool
	fetchClientOverride *fetch.Client
}

func makeApplicationServices(ctx context.Context, db *sql.DB, opts appServiceOptions) (*applicationServices, error) {
	settingsRepo := sqlite.NewSettingsRepository(db)
	subRepo := sqlite.NewSubscriptionRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	nodeSourceRepo := sqlite.NewNodeSourceRepository(db)
	fetchRepo := sqlite.NewSubscriptionFetchRepository(db)
	auditRepo := sqlite.NewAuditRepository(db)
	probeRunRepo := sqlite.NewProbeRunRepository(db)
	probeObsRepo := sqlite.NewProbeObservationRepository(db)
	policyRepo := sqlite.NewPolicyRepository(db)
	revisionRepo := sqlite.NewRevisionRepository(db)
	pubRepo := sqlite.NewPublicationRepository(db)
	riskPolicyRepo := sqlite.NewRiskPolicyRevisionRepository(db)
	riskBindingRepo := sqlite.NewRiskPolicyGroupBindingRepository(db)
	riskObsRepo := sqlite.NewIPRiskObservationRepository(db)
	probeScheduleRepo := sqlite.NewProbeScheduleRepository(db)
	nodeFilterRepo := sqlite.NewNodeFilterRepository(db)
	pubPayloadRefRepo := sqlite.NewPublicationPayloadRefRepository(db)

	var fetchClient *fetch.Client
	if opts.fetchClientOverride != nil {
		fetchClient = opts.fetchClientOverride
	} else {
		fetchPolicy := fetch.DefaultPolicy()
		if opts.fetchProxy != "" {
			if err := fetchPolicy.AddAllowedProxy(opts.fetchProxy); err != nil {
				fmt.Fprintf(os.Stderr, "warning: invalid CSP_FETCH_PROXY %q: %v\n", opts.fetchProxy, err)
			}
		}
		fetchClient = fetch.NewClientWithPolicy(fetchPolicy)
	}

	subService := subscription.NewService(subRepo, auditRepo)
	invOpts := []inventory.Option{
		inventory.WithProbeObservationRepository(probeObsRepo),
		inventory.WithDefaultFetchProxy(opts.fetchProxy),
	}
	invService := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, nodeSourceRepo, fetchClient, invOpts...)
	subService.SetReconciler(invService)

	probeScheduler, err := queue.NewScheduler(queue.Config{
		Concurrency: queue.DefaultConcurrency,
		Context:     ctx,
	})
	if err != nil {
		return nil, fmt.Errorf("probe scheduler initialization failed: %w", err)
	}
	invService.SetNodePoolStateProvider(probeScheduler)

	var runnerOpts []probe.DefaultRunnerOption
	runnerOpts = append(runnerOpts, probe.WithIPRiskObservationRepository(riskObsRepo))
	if len(opts.probeRunnerOpts) > 0 {
		runnerOpts = append(runnerOpts, opts.probeRunnerOpts...)
	}

	var probeRunner probe.Runner
	if opts.newProbeRunner != nil {
		probeRunner = opts.newProbeRunner(db, nodeRepo, probeObsRepo, probeScheduler, probeRunRepo, runnerOpts...)
	} else {
		probeRunner = probe.NewDefaultRunner(nodeRepo, probeObsRepo, probeScheduler, probeRunRepo, runnerOpts...)
	}

	probeService := probe.NewService(
		probeRunRepo,
		probe.WithRunner(probeRunner),
		probe.WithNodeRepository(nodeRepo),
		probe.WithObservationRepository(probeObsRepo),
		probe.WithScheduler(probeScheduler),
		probe.WithScheduleRepository(probeScheduleRepo),
		probe.WithAudit(auditRepo),
	)

	periodicCoordinator := probe.NewPeriodicCoordinator(
		probeScheduleRepo,
		nodeRepo,
		probeRunRepo,
		probeRunner,
		probe.WithCoordinatorOwner("csp-instance-"+domain.MustNewUUIDv7()),
		probe.WithCoordinatorObservations(probeObsRepo),
	)
	probeService.SetCoordinator(periodicCoordinator)

	if opts.startCoordinator {
		if err := periodicCoordinator.Recover(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "serve: probe periodic recovery warning: %v\n", err)
		}
		periodicCoordinator.Start(ctx)
	}

	policyService := policy.NewService(policyRepo, revisionRepo, nodeRepo, auditRepo, nodeFilterRepo)
	revisionService := revision.NewService(revisionRepo, auditRepo, revision.WithPolicyRepository(policyRepo))
	if _, err := policyService.EnsureActiveRevision(ctx); err != nil {
		return nil, fmt.Errorf("failed to ensure initial active configuration revision: %w", err)
	}

	ipriskService := iprisk.NewService(
		riskObsRepo,
		riskPolicyRepo,
		iprisk.WithBindingRepository(riskBindingRepo),
		iprisk.WithGroupRepository(policyRepo),
		iprisk.WithNodeRepository(nodeRepo),
		iprisk.WithAuditRepository(auditRepo),
	)

	pubOpts := []publication.Option{
		publication.WithPolicyRepository(policyRepo),
		publication.WithRevisionRepository(revisionRepo),
		publication.WithNodeRepository(nodeRepo),
		publication.WithNodeFilterRepository(nodeFilterRepo),
		publication.WithNodeSourceRepository(nodeSourceRepo),
		publication.WithProbeObservationRepository(probeObsRepo),
		publication.WithIPRiskService(ipriskService),
		publication.WithRiskPolicyRepository(riskPolicyRepo),
		publication.WithRiskBindingRepository(riskBindingRepo),
		publication.WithRiskObservationRepository(riskObsRepo),
		publication.WithPayloadRefRepository(pubPayloadRefRepo),
	}
	pubService := publication.NewService(
		pubRepo,
		auditRepo,
		pubOpts...,
	)

	return &applicationServices{
		settingsRepo:        settingsRepo,
		subRepo:             subRepo,
		nodeRepo:            nodeRepo,
		nodeSourceRepo:      nodeSourceRepo,
		fetchRepo:           fetchRepo,
		auditRepo:           auditRepo,
		probeRunRepo:        probeRunRepo,
		probeObsRepo:        probeObsRepo,
		policyRepo:          policyRepo,
		revisionRepo:        revisionRepo,
		pubRepo:             pubRepo,
		riskPolicyRepo:      riskPolicyRepo,
		riskBindingRepo:     riskBindingRepo,
		riskObsRepo:         riskObsRepo,
		probeScheduleRepo:   probeScheduleRepo,
		nodeFilterRepo:      nodeFilterRepo,
		pubPayloadRefRepo:   pubPayloadRefRepo,
		fetchClient:         fetchClient,
		subService:          subService,
		invService:          invService,
		probeScheduler:      probeScheduler,
		probeRunner:         probeRunner,
		probeService:        probeService,
		periodicCoordinator: periodicCoordinator,
		policyService:       policyService,
		revisionService:     revisionService,
		ipriskService:       ipriskService,
		pubService:          pubService,
	}, nil
}
