package accountapi

import (
	"context"
	"html/template"
	"net/http"
	"time"

	"professional-information-repo/internal/accountstore"
)

type statusLanguage struct {
	Title, Impact, State, Next, Updated, Unavailable, Limited string
	Labels                                                    map[string]string
}

var statusLanguages = map[string]statusLanguage{
	"en": {"CareerOS service status", "Confirmed impact", "Current status", "Next update", "Last updated",
		"Status information is unavailable.", "The pilot remains limited.",
		map[string]string{"service": "CareerOS service", "invitations_and_inference": "Invitations, activation and AI requests", "inference": "AI requests",
			"investigating": "Investigating", "paused": "Paused", "recovering": "Recovering", "recovered": "Verified recovery"}},
	"pt-BR": {"Status do serviço CareerOS", "Impacto confirmado", "Status atual", "Próxima atualização", "Última atualização",
		"As informações de status estão indisponíveis.", "O piloto continua limitado.",
		map[string]string{"service": "Serviço CareerOS", "invitations_and_inference": "Convites, ativação e solicitações de IA", "inference": "Solicitações de IA",
			"investigating": "Em investigação", "paused": "Em pausa", "recovering": "Em recuperação", "recovered": "Recuperação verificada"}},
}

var pilotStatusPage = template.Must(template.New("status").Parse(`<!doctype html>
<html lang="{{.Locale}}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>{{.Language.Title}}</title></head>
<body><main><h1>{{.Language.Title}}</h1>
{{if .Available}}<dl><dt>{{.Language.Impact}}</dt><dd>{{.Impact}}</dd>
<dt>{{.Language.State}}</dt><dd>{{.State}}</dd>
{{if .Next}}<dt>{{.Language.Next}}</dt><dd><time datetime="{{.Next}}">{{.Next}}</time></dd>{{end}}
<dt>{{.Language.Updated}}</dt><dd><time datetime="{{.Published}}">{{.Published}}</time></dd></dl>
{{if .Limited}}<p>{{.Language.Limited}}</p>{{end}}
{{else}}<p>{{.Language.Unavailable}}</p>{{end}}
<nav aria-label="Language"><a lang="en" href="/status/en">English</a> · <a lang="pt-BR" href="/status/pt-BR">Português</a></nav>
</main></body></html>`))

// NewPilotPublicStatus is a standalone read-only local handler. It has no
// application router, session, database connection or account configuration.
type PilotPublicStatusReader interface {
	PublicStatus(context.Context) (accountstore.OperationsPublicStatus, error)
}

func NewPilotPublicStatus(journal PilotPublicStatusReader) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status/{locale}", func(w http.ResponseWriter, r *http.Request) {
		locale := r.PathValue("locale")
		language, ok := statusLanguages[locale]
		if !ok {
			http.NotFound(w, r)
			return
		}
		data := struct {
			Locale                         string
			Language                       statusLanguage
			Available, Limited             bool
			Impact, State, Next, Published string
		}{Locale: locale, Language: language}
		if journal != nil {
			update, err := journal.PublicStatus(r.Context())
			if err == nil {
				data.Available = true
				data.Impact, data.State = language.Labels[update.Impact], language.Labels[update.Status]
				data.Published = update.PublishedAt.UTC().Format(time.RFC3339)
				if update.NextUpdateAt != nil {
					data.Next = update.NextUpdateAt.UTC().Format(time.RFC3339)
				}
				data.Limited = update.Limitation == "limited_pilot"
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if !data.Available {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = pilotStatusPage.Execute(w, data)
	})
	return mux
}
