package server

import (
	"context"
	"fmt"
	"sync"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe"
	"clash-sub-parser/internal/repository"
)

type ProbeSummary struct {
	Total   int `json:"total"`
	OK      int `json:"ok"`
	Fail    int `json:"fail"`
	Timeout int `json:"timeout"`
	Skipped int `json:"skipped"`
}

type ProbeStatusResponse struct {
	State           string        `json:"state"`
	ServerNow       int64         `json:"server_now"`
	IntervalMinutes int           `json:"interval_minutes"`
	NextExpectedAt  *int64        `json:"next_expected_at"`
	LastStartedAt   *int64        `json:"last_started_at"`
	LastFinishedAt  *int64        `json:"last_finished_at"`
	LastSummary     *ProbeSummary `json:"last_summary"`
	LastErrorCode   *string       `json:"last_error_code"`
}

type ProbeManager struct {
	mu             sync.RWMutex
	repos          *repository.Repositories
	engine         probe.Engine
	state          string
	lastStartedAt  *int64
	lastFinishedAt *int64
	lastSummary    *ProbeSummary
	lastErrorCode  *string
	cancelCurrent  context.CancelFunc
}

func newProbeManager(repos *repository.Repositories, engine probe.Engine) *ProbeManager {
	return &ProbeManager{
		repos:  repos,
		engine: engine,
		state:  "waiting",
	}
}

func (m *ProbeManager) GetStatus() ProbeStatusResponse {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return ProbeStatusResponse{
		State:           m.state,
		ServerNow:       time.Now().Unix(),
		IntervalMinutes: 60,
		NextExpectedAt:  nil,
		LastStartedAt:   m.lastStartedAt,
		LastFinishedAt:  m.lastFinishedAt,
		LastSummary:     m.lastSummary,
		LastErrorCode:   m.lastErrorCode,
	}
}

// Cancel stops any currently running probe execution batch.
func (m *ProbeManager) Cancel() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancelCurrent != nil {
		m.cancelCurrent()
	}
}

func (m *ProbeManager) Start(nodes []*domain.Node, opts probe.Options) error {
	m.mu.Lock()
	if m.state == "running" {
		m.mu.Unlock()
		return fmt.Errorf("probe job is already running")
	}

	m.state = "running"
	now := time.Now().Unix()
	m.lastStartedAt = &now
	m.lastErrorCode = nil

	ctx, cancel := context.WithCancel(context.Background())
	m.cancelCurrent = cancel
	m.mu.Unlock()

	go func() {
		defer cancel()

		summary := &ProbeSummary{
			Total: len(nodes),
		}

		if m.engine == nil {
			m.mu.Lock()
			m.state = "waiting"
			fin := time.Now().Unix()
			m.lastFinishedAt = &fin
			m.lastSummary = summary
			m.mu.Unlock()
			return
		}

		resChan, err := m.engine.ProbeBatch(ctx, nodes, opts)
		if err != nil {
			m.mu.Lock()
			m.state = "failed"
			errStr := err.Error()
			m.lastErrorCode = &errStr
			fin := time.Now().Unix()
			m.lastFinishedAt = &fin
			m.mu.Unlock()
			return
		}

		for res := range resChan {
			if res == nil {
				continue
			}
			switch res.Status {
			case domain.ProbeStatusOK:
				summary.OK++
			case domain.ProbeStatusTimeout:
				summary.Timeout++
			case domain.ProbeStatusFailed, domain.ProbeStatusError:
				summary.Fail++
			default:
				summary.Skipped++
			}

			// Persist probe observation
			_ = m.repos.Probes.Upsert(context.Background(), res)
		}

		m.mu.Lock()
		m.state = "waiting"
		fin := time.Now().Unix()
		m.lastFinishedAt = &fin
		m.lastSummary = summary
		m.mu.Unlock()
	}()

	return nil
}
