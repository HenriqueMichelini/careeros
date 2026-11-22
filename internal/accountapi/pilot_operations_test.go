package accountapi_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/testsupport/aiadmission"
)

func TestPilotOperationsActualExpenseWarningAndSupportBoundary(t *testing.T) {
	f, h, provider, router := qualificationFixture(t)
	ctx := context.Background()
	response := qualificationRequest(t, f, router, "/api/qualification-gaps", "operations-warning", "", f.Body(t, "/api/qualification-gaps"))
	if response.Code != http.StatusOK || len(provider.Calls) != 3 {
		t.Fatalf("actual managed source: %d calls=%d", response.Code, len(provider.Calls))
	}
	var delivered []accountstore.OperationsNotification
	journalDirectory := t.TempDir()
	if err := os.Chmod(journalDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	journal, err := accountstore.OpenLocalOperationsJournal(accountstore.OperationsJournalConfig{
		Directory: journalDirectory, BindingKey: sha256.Sum256([]byte("synthetic-operations-journal-key")),
		Notify: func(_ context.Context, notice accountstore.OperationsNotification) accountstore.OperationsDelivery {
			delivered = append(delivered, notice)
			return accountstore.OperationsDelivered
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ops, err := f.Store.LocalOperations(accountstore.OperationsConfig{Build: "synthetic-build", Owner: "Henrique"}, h.LocalOperationsSource(), journal)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Store.BootstrapLocalOperationsRole(ctx, auth.IdentityRef{Provider: "google", Issuer: "https://accounts.google.com", Subject: "fixture-account"}, "owner"); err != nil {
		t.Fatal(err)
	}
	if err = ops.Inspect(ctx); err != nil {
		t.Fatal(err)
	}
	if err = ops.Deliver(ctx); err != nil {
		t.Fatal(err)
	}
	status, err := ops.OwnerStatus(ctx, f.Principal)
	if err != nil {
		t.Fatal(err)
	}
	warnings, warningDeliveries := 0, 0
	for _, incident := range status.Incidents {
		if incident.Kind == "spend_warning" {
			warnings++
			// Three selected legs count 1,344 + 12,000 + 12,000, and
			// the qualification attempt's selected 30,000 cap warns at 24,000.
			if incident.Exposure != 25344 || incident.Cap != 30000 || incident.Threshold != 24000 || incident.Scope != "attempt" {
				t.Fatalf("independently expected warning: %+v", incident)
			}
		}
	}
	for _, notice := range delivered {
		if notice.Kind == "spend_warning" {
			warningDeliveries++
		}
	}
	if warnings != 1 || warningDeliveries != 1 {
		t.Fatalf("one original warning: records=%d deliveries=%d", warnings, warningDeliveries)
	}
	beforeDeliveryCount := len(delivered)
	if err = ops.Inspect(ctx); err != nil {
		t.Fatal(err)
	}
	if err = ops.Deliver(ctx); err != nil {
		t.Fatal(err)
	}
	for _, notice := range delivered[beforeDeliveryCount:] {
		if notice.Kind == "spend_warning" {
			t.Fatal("unchanged inspection resent warning")
		}
	}
	support := qualificationParticipant(t, f, "operations-support")
	if err = f.Store.BootstrapLocalOperationsRole(ctx, auth.IdentityRef{Provider: "google", Issuer: "https://accounts.google.com", Subject: "operations-support"}, "support"); err != nil {
		t.Fatal(err)
	}
	if _, err = ops.OwnerStatus(ctx, support.Principal); err != auth.ErrForbidden {
		t.Fatalf("support obtained restricted risk summary: %v", err)
	}
	req := httptest.NewRequest("GET", "/api/profile/current", nil)
	req.AddCookie(&http.Cookie{Name: "__Host-careeros_session", Value: support.Login.SessionToken})
	req.Header.Set("X-CareerOS-Account", f.Principal.AccountID)
	w := httptest.NewRecorder()
	accountapi.New(f.Service, f.Store).ServeHTTP(w, req)
	if w.Code != http.StatusConflict || bytes.Contains(w.Body.Bytes(), []byte("document")) {
		t.Fatalf("routine support browsed another stored Profile: %d", w.Code)
	}
}

func TestPilotOperationsIndependentControlFaultRetriesFromOriginalAttempt(t *testing.T) {
	f, h, provider, router := qualificationFixture(t)
	ctx := context.Background()
	at := time.Date(2026, 11, 21, 5, 0, 0, 0, time.UTC)
	now := at
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	var notices []accountstore.OperationsNotification
	cfg := accountstore.OperationsJournalConfig{
		Directory: directory, BindingKey: sha256.Sum256([]byte("synthetic-outage-journal")),
		Clock: func() time.Time { return now },
		Notify: func(_ context.Context, notice accountstore.OperationsNotification) accountstore.OperationsDelivery {
			notices = append(notices, notice)
			return accountstore.OperationsFailed
		},
	}
	open := func() *accountstore.Operations {
		journal, err := accountstore.OpenLocalOperationsJournal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		ops, err := f.Store.LocalOperations(accountstore.OperationsConfig{Build: "synthetic-build", Owner: "Henrique"}, h.LocalOperationsSource(), journal)
		if err != nil {
			t.Fatal(err)
		}
		return ops
	}
	ops := open()
	if err := ops.ObserveFault(ctx, accountstore.OperationsControlFault); err != nil {
		t.Fatal(err)
	}
	if len(notices) != 1 || notices[0].Kind != "control_fault" || notices[0].Owner != "Henrique" || notices[0].Response == "" {
		t.Fatalf("first fault requires an actionable owner notice: %+v", notices)
	}
	w := qualificationRequest(t, f, router, "/api/qualification-gaps", "after-operations-fault", "", f.Body(t, "/api/qualification-gaps"))
	if w.Code == http.StatusOK || len(provider.Calls) != 0 {
		t.Fatalf("independent observer did not stop protected inference: %d calls=%d", w.Code, len(provider.Calls))
	}
	ops = open()
	for _, step := range []struct {
		elapsed time.Duration
		want    int
	}{
		{4*time.Minute + 59*time.Second, 1}, {5 * time.Minute, 2}, {29*time.Minute + 59*time.Second, 2},
		{30 * time.Minute, 3}, {24 * time.Hour, 3},
	} {
		now = at.Add(step.elapsed)
		if err := ops.ObserveFault(ctx, accountstore.OperationsControlFault); err != nil {
			t.Fatal(err)
		}
		if len(notices) != step.want || notices[len(notices)-1].ID != notices[0].ID {
			t.Fatalf("at %s got %d notices, want %d for the original incident", step.elapsed, len(notices), step.want)
		}
	}
}

func TestPilotOperationsOrdinaryCaseAndDiagnosticExpiry(t *testing.T) {
	f, h, _, _ := qualificationFixture(t)
	ctx := context.Background()
	at := time.Date(2026, 11, 21, 5, 0, 0, 0, time.UTC) // Saturday.
	now := at
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	j, err := accountstore.OpenLocalOperationsJournal(accountstore.OperationsJournalConfig{
		Directory: directory, BindingKey: sha256.Sum256([]byte("synthetic-support-journal")),
		Clock: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	ops, err := f.Store.LocalOperations(accountstore.OperationsConfig{Build: "synthetic-build", Owner: "Henrique"}, h.LocalOperationsSource(), j)
	if err != nil {
		t.Fatal(err)
	}
	support := qualificationParticipant(t, f, "ordinary-support")
	if err = f.Store.BootstrapLocalOperationsRole(ctx, auth.IdentityRef{Provider: "google", Issuer: "https://accounts.google.com", Subject: "ordinary-support"}, "support"); err != nil {
		t.Fatal(err)
	}
	if err = f.Store.BootstrapLocalOperationsRole(ctx, auth.IdentityRef{Provider: "google", Issuer: "https://accounts.google.com", Subject: "fixture-account"}, "owner"); err != nil {
		t.Fatal(err)
	}
	c, err := ops.CreateCase(ctx, support.Principal, accountstore.OperationsCaseInput{Purpose: "product", Locale: "pt-BR",
		Message: "Como posso encerrar minha sessão?", Example: "Exemplo de tela fictícia.", ExampleKind: "synthetic"})
	if err != nil || c.Message != "Como posso encerrar minha sessão?" || c.Status != "received" ||
		c.FirstResponseDue.Format(time.RFC3339) != "2026-11-23T05:00:00Z" || c.NecessityReviewAt.Format(time.RFC3339) != "2026-11-28T05:00:00Z" {
		t.Fatalf("Saturday receipt and internal targets: %+v %v", c, err)
	}
	if _, err := ops.CreateCase(ctx, support.Principal, accountstore.OperationsCaseInput{Purpose: "rights", Locale: "en", Message: "Synthetic deletion request."}); err != accountstore.ErrOperationsSpecialPurpose {
		t.Fatalf("rights request was classified as ordinary: %v", err)
	}
	if err := ops.RecordDiagnostic(ctx, accountstore.OperationsAuthFault); err != nil {
		t.Fatal(err)
	}
	now = at.Add(12 * time.Hour)
	if err := ops.UpdateCase(ctx, support.Principal, c.ID, "respond", "Abra o menu e selecione Sair."); err != nil {
		t.Fatal(err)
	}
	now = at.Add(24 * time.Hour)
	if err := ops.UpdateCase(ctx, support.Principal, c.ID, "close", ""); err != nil {
		t.Fatal(err)
	}
	now = at.Add(7*time.Hour*24 - time.Second)
	if err := ops.UpdateCase(ctx, support.Principal, c.ID, "close", ""); err != nil {
		t.Fatal(err)
	}
	diagnostics, err := ops.OwnerDiagnostics(ctx, f.Principal)
	if err != nil || len(diagnostics) != 1 || diagnostics[0].Kind != "auth_fault" {
		t.Fatalf("diagnostic before original event TTL: %+v %v", diagnostics, err)
	}
	now = at.Add(7 * time.Hour * 24)
	if err := ops.PruneOrdinaryRecords(ctx, f.Principal); err != nil {
		t.Fatal(err)
	}
	diagnostics, err = ops.OwnerDiagnostics(ctx, f.Principal)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("diagnostic at TTL: %+v %v", diagnostics, err)
	}
	now = at.Add(31*24*time.Hour - time.Second)
	got, err := ops.Case(ctx, support.Principal, c.ID)
	if err != nil || got.ClosedAt == nil || got.ClosedAt.Format(time.RFC3339) != "2026-11-22T05:00:00Z" ||
		got.RespondedAt == nil || got.RespondedAt.Format(time.RFC3339) != "2026-11-21T17:00:00Z" {
		t.Fatalf("original response and closure preserved: %+v %v", got, err)
	}
	now = at.Add(31 * 24 * time.Hour)
	if err := ops.PruneOrdinaryRecords(ctx, f.Principal); err != nil {
		t.Fatal(err)
	}
	if _, err := ops.Case(ctx, support.Principal, c.ID); err != accountstore.ErrNotFound {
		t.Fatalf("case present exactly at 30-day closure TTL: %v", err)
	}
	now = time.Date(2026, 12, 25, 13, 0, 0, 0, time.UTC)
	holiday, err := ops.CreateCase(ctx, support.Principal, accountstore.OperationsCaseInput{
		Purpose: "account", Locale: "en", Message: "How can I view my access end time?",
	})
	if err != nil || holiday.FirstResponseDue.Format(time.RFC3339) != "2026-12-27T13:00:00Z" || holiday.RespondedAt != nil {
		t.Fatalf("holiday receipt must retain the internal elapsed-time target: %+v %v", holiday, err)
	}
	if err = ops.ReviewCaseHold(ctx, support.Principal, holiday.ID, "privacy", now.Add(time.Hour), false); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("ordinary support installed a privileged investigation hold: %v", err)
	}
	if err = ops.ReviewCaseHold(ctx, f.Principal, holiday.ID, "privacy", now.Add(7*24*time.Hour), false); err != nil {
		t.Fatal(err)
	}
	if err = ops.UpdateCase(ctx, support.Principal, holiday.ID, "close", ""); err != nil {
		t.Fatal(err)
	}
	now = now.Add(30 * 24 * time.Hour)
	if err = ops.PruneOrdinaryRecords(ctx, f.Principal); err != nil {
		t.Fatal(err)
	}
	if _, err = ops.Case(ctx, support.Principal, holiday.ID); err != accountstore.ErrNotFound {
		t.Fatalf("held case stayed in ordinary scope: %v", err)
	}
	if err = ops.WithCase(ctx, f.Principal, holiday.ID, true, func(retained accountstore.OperationsCase) error {
		if retained.Message != "How can I view my access end time?" {
			t.Fatalf("held necessary request lost: %+v", retained)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err = ops.ReviewCaseHold(ctx, f.Principal, holiday.ID, "", time.Time{}, true); err != nil {
		t.Fatal(err)
	}
	if err = ops.PruneOrdinaryRecords(ctx, f.Principal); err != nil {
		t.Fatal(err)
	}
	if err = ops.WithCase(ctx, f.Principal, holiday.ID, true, func(accountstore.OperationsCase) error { return nil }); err != accountstore.ErrNotFound {
		t.Fatalf("retired hold still visible: %v", err)
	}
}

func TestPilotOperationsPhysicalOutageKeepsStatusAndContainmentIndependent(t *testing.T) {
	dsn, pgctl, data, log := privateRestartPostgres(t)
	t.Setenv("CAREEROS_TEST_DATABASE_URL", dsn)
	f, h, provider, router := qualificationFixture(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	j, ops, notices := pilotOperationsFixture(t, f, h, func() time.Time { return now })
	if err := j.PublishOutage(ctx, "service", "investigating", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	restartCommand(t, pgctl, "-D", data, "-m", "immediate", "-w", "stop")
	databaseURL, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	restart := func() {
		restartCommand(t, pgctl, "-D", data, "-l", log, "-o",
			fmt.Sprintf("-h 127.0.0.1 -k %s -p %s", filepath.Join(filepath.Dir(data), "socket"), databaseURL.Port()), "-w", "start")
	}
	t.Cleanup(func() {
		if _, err := os.Stat(data + "/postmaster.pid"); os.IsNotExist(err) {
			restart()
		}
	})
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	err = ops.Inspect(bounded)
	cancel()
	if err == nil || len(*notices) != 1 || (*notices)[0].Kind != "database_fault" {
		t.Fatalf("database failure did not use independent notification: %v %+v", err, *notices)
	}
	authFault := httptest.NewRequest("GET", "/api/auth/session", nil)
	authFault.AddCookie(&http.Cookie{Name: "__Host-careeros_session", Value: f.Login.SessionToken})
	authResponse := httptest.NewRecorder()
	accountapi.NewPilot(f.Service, f.Store, nil).ServeHTTP(authResponse, authFault)
	if authResponse.Code != http.StatusServiceUnavailable || len(*notices) != 2 || (*notices)[1].Kind != "auth_fault" {
		t.Fatalf("unexpected auth storage failure not observed: %d %+v", authResponse.Code, *notices)
	}
	now = now.Add(time.Hour)
	if err = j.PublishOutage(ctx, "service", "investigating", now.Add(30*time.Minute)); err != nil {
		t.Fatal(err)
	}
	for path, label := range map[string]string{"/status/en": "Confirmed impact", "/status/pt-BR": "Impacto confirmado"} {
		w := httptest.NewRecorder()
		accountapi.NewPilotPublicStatus(j).ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), label) || !strings.Contains(w.Body.String(), "2026-11-22T11:30:00Z") ||
			strings.Contains(w.Body.String(), f.Principal.AccountID) || strings.Contains(w.Body.String(), (*notices)[0].ID) {
			t.Fatalf("independent public page disclosed private facts or lost update: %d %s", w.Code, w.Body.String())
		}
	}
	restart()
	response := qualificationRequest(t, f, router, "/api/qualification-gaps", "before-journal-replay", "", f.Body(t, "/api/qualification-gaps"))
	if response.Code == 200 || len(provider.Calls) != 0 {
		t.Fatalf("restart before replay: %d calls %d", response.Code, len(provider.Calls))
	}
	if err = ops.Inspect(ctx); err != nil {
		t.Fatal(err)
	}
	s, err := ops.OwnerStatus(ctx, f.Principal)
	if err != nil || len(s.Incidents) != 2 || len(*notices) != 2 {
		t.Fatalf("same incidents survived primary recovery without duplicate: %+v %v", s, err)
	}
	controls, err := f.Store.LocalControls(nil).Status(ctx, f.ControlOperator.Principal, "")
	if err != nil || !controls.InferenceStopped || !controls.InvitationStopped {
		t.Fatalf("independent journal failed to replay to original authority: %+v %v", controls, err)
	}
}

func TestPilotOperationsActualUnknownAndBreachRemainSeparate(t *testing.T) {
	for _, c := range []struct {
		name, want      string
		exposure, limit int64
	}{{"unknown", "unknown_dispatch", 1344, 1344}, {"breach", "maximum_breach", 2688, 1344}} {
		t.Run(c.name, func(t *testing.T) {
			f, h, provider, router := qualificationFixture(t)
			_, ops, notices := pilotOperationsFixture(t, f, h, time.Now)
			calls := 0
			err := h.SetQualificationResponder(func(ctx context.Context, name string, r *http.Request) (*http.Response, bool, error) {
				calls++
				if c.name == "unknown" {
					return nil, false, errors.New("synthetic provider timeout")
				}
				response, e := provider.Respond(ctx, name, r)
				if e != nil {
					return response, false, e
				}
				raw, e := io.ReadAll(response.Body)
				response.Body.Close()
				if e != nil {
					return nil, false, e
				}
				var body map[string]any
				if json.Unmarshal(raw, &body) != nil {
					t.Fatal("fake response")
				}
				body["model"] = "jev-1.13.0"
				body["service_tier"] = "standard"
				body["usage"] = map[string]int64{"input_tokens": 64000, "output_tokens": 0}
				raw, _ = json.Marshal(body)
				response.Body = io.NopCloser(bytes.NewReader(raw))
				response.Header.Set("X-Request-ID", "test_above_bound")
				return response, true, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			w := qualificationRequest(t, f, router, "/api/qualification-gaps", "operations-"+c.name, "", f.Body(t, "/api/qualification-gaps"))
			if w.Code == 200 || calls != 1 {
				t.Fatalf("fault outcome code %d calls %d", w.Code, calls)
			}
			if err = ops.Inspect(context.Background()); err != nil {
				t.Fatal(err)
			}
			status, err := ops.OwnerStatus(context.Background(), f.Principal)
			if err != nil || status.Figures == nil || status.Figures.Counted != c.exposure {
				t.Fatalf("gross figures: %+v %v", status, err)
			}
			found, delivered := 0, 0
			for _, event := range status.Incidents {
				if event.Kind == c.want {
					found++
					if event.Exposure != c.exposure || event.Cap != c.limit {
						t.Fatalf("event figures: %+v", event)
					}
				}
			}
			for _, n := range *notices {
				if n.Kind == c.want {
					delivered++
				}
			}
			if found != 1 || delivered != 1 {
				t.Fatalf("expected one %s, got %+v %+v", c.want, status.Incidents, *notices)
			}
			if c.name == "unknown" && (status.Figures.PossibleRemote != 1 || status.Figures.UnknownDifference != 1344) {
				t.Fatalf("unknown financial and remote holds: %+v", status.Figures)
			}
			before := len(*notices)
			if err = ops.Inspect(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(*notices) != before {
				t.Fatal("duplicate inspection resent a notice")
			}
		})
	}
}

func pilotOperationsFixture(t *testing.T, f *aiadmission.Fixture, h *aicontrol.Harness, now func() time.Time) (*accountstore.OperationsJournal, *accountstore.Operations, *[]accountstore.OperationsNotification) {
	t.Helper()
	ctx := context.Background()
	var notices []accountstore.OperationsNotification
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	j, err := accountstore.OpenLocalOperationsJournal(accountstore.OperationsJournalConfig{Directory: directory,
		BindingKey: sha256.Sum256([]byte("synthetic-operations-integration")), Clock: now,
		Notify: func(_ context.Context, n accountstore.OperationsNotification) accountstore.OperationsDelivery {
			notices = append(notices, n)
			return accountstore.OperationsDelivered
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ops, err := f.Store.LocalOperations(accountstore.OperationsConfig{Build: "synthetic-build", Owner: "Henrique"}, h.LocalOperationsSource(), j)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Store.BootstrapLocalOperationsRole(ctx, auth.IdentityRef{Provider: "google", Issuer: "https://accounts.google.com", Subject: "fixture-account"}, "owner"); err != nil {
		t.Fatal(err)
	}
	return j, ops, &notices
}

func TestPilotOperationsWarningAbsenceAndVerifiedReopening(t *testing.T) {
	f, h, _, router := qualificationFixture(t)
	ctx := context.Background()
	now := time.Now()
	j, ops, _ := pilotOperationsFixture(t, f, h, func() time.Time { return now })
	body := f.Body(t, "/api/qualification-gaps")
	w := qualificationRequest(t, f, router, "/api/qualification-gaps", "operations-review", "", body)
	if w.Code != 200 {
		t.Fatalf("qualify before warning: %d", w.Code)
	}
	if err := ops.Inspect(ctx); err != nil {
		t.Fatal(err)
	}
	status, err := ops.OwnerStatus(ctx, f.Principal)
	if err != nil || len(status.Incidents) != 1 || status.Incidents[0].Kind != "spend_warning" {
		t.Fatalf("warning: %+v %v", status, err)
	}
	warn := status.Incidents[0]
	controls := f.Store.LocalControls(nil)
	operator := f.ControlOperator.Principal
	now = warn.EventAt.Add(24*time.Hour - time.Second)
	if err = ops.Inspect(ctx); err != nil {
		t.Fatal(err)
	}
	pre, err := controls.Status(ctx, operator, "")
	if err != nil || pre.InferenceStopped || pre.InvitationStopped {
		t.Fatalf("early containment: %+v %v", pre, err)
	}
	now = warn.EventAt.Add(24 * time.Hour)
	if err = ops.Inspect(ctx); err != nil {
		t.Fatal(err)
	}
	stopped, err := controls.Status(ctx, operator, "")
	if err != nil || !stopped.InferenceStopped || !stopped.InvitationStopped {
		t.Fatalf("missing exact 24h containment: %+v %v", stopped, err)
	}
	if err = ops.ReviewIncident(ctx, f.Principal, warn.ID, warn.Revision, false); err != nil {
		t.Fatal(err)
	}
	verified, err := aicontrol.NewSyntheticControlRecovery(ctx, f.Store, h)
	if err != nil {
		t.Fatal(err)
	}
	command := accountstore.ControlCommand{Control: "inference", ExpectedVersion: stopped.Version}
	command.RecoveryReference, err = verified.Prepare(ctx, operator, command)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.Store.LocalControls(verified).Apply(ctx, operator, "still-unresolved", command); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("notification acknowledgment opened control: %v", err)
	}
	if err = ops.ReviewIncident(ctx, f.Principal, warn.ID, warn.Revision, true); err != nil {
		t.Fatal(err)
	}
	if err = ops.ObserveFault(ctx, accountstore.OperationsBackupFault); err != nil {
		t.Fatal(err)
	}
	if _, err = f.Store.LocalControls(verified).Apply(ctx, operator, "older-reviewed-recovery", command); err == nil {
		t.Fatal("new backup incident failed to invalidate prior proof")
	}
	status, err = ops.OwnerStatus(ctx, f.Principal)
	if err != nil || status.Backup != "declared_failure" {
		t.Fatalf("backup health: %+v %v", status, err)
	}
	for _, incident := range status.Incidents {
		if incident.Kind == "backup_fault" {
			if err = ops.ReviewIncident(ctx, f.Principal, incident.ID, incident.Revision, true); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err = j.PublishOutage(ctx, "invitations_and_inference", "paused", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	var lastReference string
	for _, control := range []string{"inference", "invitations"} {
		current, err := controls.Status(ctx, operator, "")
		if err != nil {
			t.Fatal(err)
		}
		reopen := accountstore.ControlCommand{Control: control, ExpectedVersion: current.Version}
		reopen.RecoveryReference, err = verified.Prepare(ctx, operator, reopen)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Store.LocalControls(verified).Apply(ctx, operator, "reviewed-"+control, reopen); err != nil {
			t.Fatal(err)
		}
		lastReference = reopen.RecoveryReference
	}
	if err = ops.PublishVerifiedRecovery(ctx, f.Principal, lastReference, "limited_pilot"); err != nil {
		t.Fatal(err)
	}
	closed, err := j.PublicStatus(ctx)
	if err != nil || closed.Status != "recovered" || closed.NextUpdateAt != nil || closed.Limitation != "limited_pilot" {
		t.Fatalf("verified closure: %+v %v", closed, err)
	}
	before, err := f.Store.PilotStatus(ctx, f.Principal)
	if err != nil {
		t.Fatal(err)
	}
	plan := accountstore.OperationsAbsence{Starts: now.Add(12 * time.Hour), Ends: now.Add(36 * time.Hour), NextUpdateAt: now.Add(24 * time.Hour)}
	r, err := ops.PlanAbsence(ctx, f.Principal, plan)
	if err != nil || !r.Paused || r.ActivePilotAllowed {
		t.Fatalf("missing prepared cover/rights owner: %+v %v", r, err)
	}
	after, err := f.Store.PilotStatus(ctx, f.Principal)
	if err != nil || !after.CanRetrieve || !after.ExpiresAt.Equal(*before.ExpiresAt) || after.DraftAllowance != before.DraftAllowance {
		t.Fatalf("absence altered independent grant clocks/retrieval: %+v %v", after, err)
	}
	stopped, err = controls.Status(ctx, operator, "")
	if err != nil || !stopped.InferenceStopped || !stopped.InvitationStopped {
		t.Fatalf("absence didn't pause before start: %+v %v", stopped, err)
	}
	public, err := j.PublicStatus(ctx)
	if err != nil || public.Status != "paused" || public.NextUpdateAt == nil || !public.NextUpdateAt.Equal(plan.NextUpdateAt) {
		t.Fatalf("absence status: %+v %v", public, err)
	}
}
