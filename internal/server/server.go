package server

import (
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"

	"clash-sub-parser/frontend"
	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/compiler/template"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe"
	"clash-sub-parser/internal/repository"
	"clash-sub-parser/internal/server/web"
)

// Server encapsulates the HTTP router, middleware pipeline, repositories, probe engine, and compiler.
type Server struct {
	router       chi.Router
	repos        *repository.Repositories
	probeEngine  probe.Engine
	probeManager *ProbeManager
	compiler     compiler.Compiler
	webFS        fs.FS
}

// NewServer initializes the chi router, middlewares, and all REST API routes with the default compiler.
func NewServer(repos *repository.Repositories, engine probe.Engine) *Server {
	comp, _ := template.NewCompiler()
	return NewServerWithCompiler(repos, engine, comp)
}

// NewServerWithCompiler initializes the server with an explicitly supplied compiler instance.
func NewServerWithCompiler(repos *repository.Repositories, engine probe.Engine, comp compiler.Compiler) *Server {
	if comp == nil {
		comp, _ = template.NewCompiler()
	}

	r := chi.NewRouter()
	setupMiddleware(r)

	probeMgr := newProbeManager(repos, engine)

	s := &Server{
		router:       r,
		repos:        repos,
		probeEngine:  engine,
		probeManager: probeMgr,
		compiler:     comp,
	}

	s.setupRoutes()

	// Automatically mount embedded frontend if available
	if distFS, err := frontend.FS(); err == nil {
		s.MountFrontend(distFS)
	}

	return s
}

// Router returns the underlying chi.Router instance.
func (s *Server) Router() chi.Router {
	return s.router
}

// Repositories returns the underlying data repositories.
func (s *Server) Repositories() *repository.Repositories {
	return s.repos
}

// ProbeManager returns the background probe manager.
func (s *Server) ProbeManager() *ProbeManager {
	return s.probeManager
}

// MountFrontend mounts the frontend SPA handler onto the router.
func (s *Server) MountFrontend(distFS fs.FS) {
	if distFS == nil {
		return
	}
	s.webFS = distFS
	spaHandler := web.NewSPAHandler(distFS)
	s.router.NotFound(spaHandler.ServeHTTP)
	s.router.Get("/", spaHandler.ServeHTTP)
}

// WebFS returns the mounted frontend filesystem, if any.
func (s *Server) WebFS() fs.FS {
	return s.webFS
}

func (s *Server) setupRoutes() {
	// Health check
	s.router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		renderJSON(w, http.StatusOK, map[string]any{
			"status": "ok",
			"engine": "go-clean-slate",
		})
	})

	// Handlers
	subH := newSubscriptionHandler(s.repos)
	nodeH := newNodeHandler(s.repos)
	groupH := newGroupHandler(s.repos)
	ruleH := newRuleHandler(s.repos)
	probeH := newProbeHandler(s.repos, s.probeManager)
	genH := newGenerateHandler(s.repos, s.compiler)

	// Root-level export shortcuts
	s.router.Get("/clash", genH.exportRoot(domain.TargetClash))
	s.router.Get("/mihomo", genH.exportRoot(domain.TargetMihomo))
	s.router.Get("/sing-box", genH.exportRoot(domain.TargetSingBox))
	s.router.Get("/surge", genH.exportRoot(domain.TargetSurge))
	s.router.Get("/loon", genH.exportRoot(domain.TargetLoon))
	s.router.Get("/stash", genH.exportRoot(domain.TargetStash))
	s.router.Get("/quantumult-x", genH.exportRoot(domain.TargetQuantumultX))
	s.router.Get("/qx", genH.exportRoot(domain.TargetQuantumultX))
	s.router.Get("/shadowrocket", genH.exportRoot(domain.TargetShadowrocket))
	s.router.Get("/yaml", genH.exportRoot(domain.TargetClash))
	s.router.Get("/script", genH.deprecatedScript)

	// API Subscriptions
	s.router.Route("/api/subscriptions", func(r chi.Router) {
		r.Get("/", subH.list)
		r.Post("/", subH.create)
		r.Get("/nodes", subH.getAllNodes)
		r.Get("/nodes/all", subH.getAllNodes)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", subH.get)
			r.Put("/", subH.update)
			r.Patch("/", subH.update)
			r.Delete("/", subH.delete)
			r.Post("/refresh", subH.refresh)
			r.Post("/fetch", subH.refresh)
			r.Get("/nodes", subH.getNodes)
		})
	})

	// API Nodes
	s.router.Route("/api/nodes", func(r chi.Router) {
		r.Get("/", nodeH.list)
		r.Post("/batch-delete", nodeH.batchDelete)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", nodeH.get)
			r.Delete("/", nodeH.delete)
		})
	})

	// API Groups & Node Groups
	mountGroups := func(r chi.Router) {
		r.Get("/", groupH.list)
		r.Post("/", groupH.create)
		r.Post("/validate", groupH.validate)
		r.Post("/reorder", groupH.reorder)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", groupH.get)
			r.Put("/", groupH.update)
			r.Patch("/", groupH.update)
			r.Delete("/", groupH.delete)
		})
	}
	s.router.Route("/api/groups", mountGroups)
	s.router.Route("/api/node-groups", mountGroups)

	// API Rules
	s.router.Route("/api/rules", func(r chi.Router) {
		r.Get("/", ruleH.list)
		r.Post("/", ruleH.create)
		r.Post("/batch", ruleH.batch)
		r.Post("/reorder", ruleH.reorder)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", ruleH.get)
			r.Put("/", ruleH.update)
			r.Patch("/", ruleH.update)
			r.Delete("/", ruleH.delete)
		})
	})

	// API Probe
	s.router.Route("/api/probe", func(r chi.Router) {
		r.Get("/status", probeH.status)
		r.Post("/start", probeH.start)
		r.Get("/results", probeH.results)
		r.Get("/results/detail", probeH.detail)
		r.Delete("/results", probeH.clearResults)
	})

	// API Generate & Client Distributions
	s.router.Route("/api/generate", func(r chi.Router) {
		r.Get("/", genH.exportAPI)
		r.Get("/settings", genH.getSettings)
		r.Patch("/settings", genH.patchSettings)
		r.Get("/quick-export", genH.quickExport)
		r.Get("/qrcode", genH.qrcode)
		r.Get("/script", genH.deprecatedScript)
		r.Post("/script", genH.deprecatedScript)
		r.Get("/script/*", genH.deprecatedScript)

		// yaml shortcuts
		r.Get("/yaml", genH.exportRoot(domain.TargetClash))
		r.Post("/yaml", genH.generateTargetPost)
		r.Get("/yaml/current", genH.exportTargetCurrent)
		r.Get("/yaml/download", genH.exportTargetDownload)

		// single subscription
		r.Get("/subscription/{id}", genH.exportSubscription)
		r.Post("/subscription/{id}", genH.generateSubscriptionPost)

		// generic target endpoints
		r.Get("/{target}", genH.exportTargetInline)
		r.Post("/{target}", genH.generateTargetPost)
		r.Get("/{target}/current", genH.exportTargetCurrent)
		r.Get("/{target}/download", genH.exportTargetDownload)
	})

	// API QRCode shortcut
	s.router.Get("/api/qrcode", genH.qrcode)
}
