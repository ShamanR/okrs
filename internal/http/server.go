package http

import (
	"embed"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"okrs/internal/domain"
	"okrs/internal/http/handlers/api"
	"okrs/internal/http/handlers/common"
	"okrs/internal/http/handlers/goals"
	"okrs/internal/http/handlers/keyresults"
	"okrs/internal/http/handlers/teams"
	"okrs/internal/store"

	"github.com/go-chi/chi/v5"
)

//go:embed templates/*.html
var templatesFS embed.FS

type Server struct {
	store  *store.Store
	logger *slog.Logger
	tmpl   *template.Template
	zone   *time.Location
}

func NewServer(store *store.Store, logger *slog.Logger, zone *time.Location) (*Server, error) {
	tmpl, err := template.New("").Funcs(template.FuncMap{
		"stageAt": func(stages []domain.KRProjectStage, index int) *domain.KRProjectStage {
			if index < 0 || index >= len(stages) {
				return nil
			}
			stage := stages[index]
			return &stage
		},
	}).ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &Server{store: store, logger: logger, tmpl: tmpl, zone: zone}, nil
}

func (s *Server) Routes() http.Handler {
	deps := common.Dependencies{Store: s.store, Logger: s.logger, Templates: s.tmpl, Zone: s.zone}
	teamsHandler := teams.New(deps)
	goalsHandler := goals.New(deps)
	krHandler := keyresults.New(deps)
	apiHandler := api.New(deps)

	r := chi.NewRouter()

	r.Get("/teams", teamsHandler.HandleTeams)
	r.Get("/teams/new", teamsHandler.HandleNewTeam)
	r.Post("/teams", teamsHandler.HandleCreateTeam)
	r.Post("/teams/{teamID}/delete", teamsHandler.HandleDeleteTeam)
	r.Get("/teams/{teamID}/okr", teamsHandler.HandleTeamOKR)
	r.Post("/teams/{teamID}/okr", teamsHandler.HandleCreateGoal)

	r.Get("/goals/{goalID}", goalsHandler.HandleGoalDetail)
	r.Post("/goals/{goalID}/comments", goalsHandler.HandleAddGoalComment)
	r.Post("/goals/{goalID}/key-results", goalsHandler.HandleAddKeyResult)
	r.Post("/goals/{goalID}/delete", goalsHandler.HandleDeleteGoal)
	r.Post("/goals/{goalID}/update", goalsHandler.HandleUpdateGoal)
	r.Get("/goals/year", goalsHandler.HandleYearGoals)

	r.Post("/key-results/{krID}/stages", krHandler.HandleAddStage)
	r.Post("/stages/{stageID}/toggle", krHandler.HandleToggleStage)
	r.Post("/key-results/{krID}/percent", krHandler.HandleUpdatePercentCurrent)
	r.Post("/key-results/{krID}/checkpoints", krHandler.HandleAddCheckpoint)
	r.Post("/key-results/{krID}/boolean", krHandler.HandleUpdateBoolean)
	r.Post("/key-results/{krID}/comments", krHandler.HandleAddKRComment)
	r.Post("/key-results/{krID}/delete", krHandler.HandleDeleteKeyResult)
	r.Post("/key-results/{krID}/update", krHandler.HandleUpdateKeyResult)

	r.Route("/api", func(r chi.Router) {
		r.Get("/teams", apiHandler.HandleAPITeams)
		r.Get("/teams/{teamID}/goals", apiHandler.HandleAPITeamGoals)
		r.Get("/goals/{goalID}", apiHandler.HandleAPIGoal)
	})

	return r
}
