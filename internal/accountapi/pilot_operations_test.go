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
	"syscall"
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
	if err = h.SetQualificationResponder(func(ctx context.Context, name string, r *http.Request) (*http.Response, bool, error) {
		unknown := 0
		for _, notice := range delivered {
			if notice.Kind == "unknown_dispatch" {
				unknown++
			}
		}
		if unknown != len(provider.Calls)+1 {
			t.Errorf("dispatch %d lacked its durable unknown notice before fake provider entry: %d", len(provider.Calls)+1, unknown)
		}
		w, e := provider.Respond(ctx, name, r)
		return w, true, e
	}); err != nil {
		t.Fatal(err)
	}
	response := qualificationRequest(t, f, router, "/api/qualification-gaps", "operations-warning", "", f.Body(t, "/api/qualification-gaps"))
	if response.Code != http.StatusOK || len(provider.Calls) != 3 {
		t.Fatalf("actual managed source: %d calls=%d", response.Code, len(provider.Calls))
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
	now := at.Add(6 * time.Hour)
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
		Message: "Como posso encerrar minha sessão?", Example: "Exemplo de tela fictícia.", ExampleKind: "synthetic", ReceivedAt: at})
	if err != nil || c.Message != "Como posso encerrar minha sessão?" || c.Status != "received" ||
		c.ReceivedAt.Format(time.RFC3339) != "2026-11-21T05:00:00Z" || c.IntakeAt.Format(time.RFC3339) != "2026-11-21T11:00:00Z" ||
		c.FirstResponseDue.Format(time.RFC3339) != "2026-11-23T05:00:00Z" || c.NecessityReviewAt.Format(time.RFC3339) != "2026-11-28T11:00:00Z" {
		t.Fatalf("Saturday receipt and internal targets: %+v %v", c, err)
	}
	if _, err := ops.CreateCase(ctx, support.Principal, accountstore.OperationsCaseInput{Purpose: "rights", Locale: "en", Message: "Synthetic deletion request."}); err != accountstore.ErrOperationsSpecialPurpose {
		t.Fatalf("rights request was classified as ordinary: %v", err)
	}
	now = at
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
		Purpose: "account", Locale: "en", Message: "How can I view my access end time?", ReceivedAt: now,
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
	j, ops, notices, _ := pilotOperationsFixture(t, f, h, func() time.Time { return now })
	offlineKey := sha256.Sum256([]byte("synthetic-separate-offline-owner"))
	if err := ops.ProvisionOfflineOwner(ctx, f.Principal, offlineKey); err != nil {
		t.Fatal(err)
	}
	body := f.Body(t, "/api/qualification-gaps")
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
	authResponse := qualificationRequest(t, f, router, "/api/qualification-gaps", "operations-db-unavailable", "", body)
	if authResponse.Code != http.StatusServiceUnavailable || len(*notices) != 2 || (*notices)[1].Kind != "auth_fault" {
		t.Fatalf("unexpected auth storage failure not observed: %d %+v", authResponse.Code, *notices)
	}
	called := false
	if err = j.WithOfflineOwnerStatus(ctx, sha256.Sum256([]byte("synthetic-wrong-reader")), func(accountstore.OperationsStatus) error { called = true; return nil }); !errors.Is(err, auth.ErrForbidden) || called {
		t.Fatalf("observer read restricted outage list: %v", err)
	}
	if err = j.WithOfflineOwnerStatus(ctx, offlineKey, func(s accountstore.OperationsStatus) error {
		if len(s.Incidents) != 2 {
			t.Fatalf("offline owner missed the two real faults: %+v", s)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
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
			_, ops, notices, _ := pilotOperationsFixture(t, f, h, time.Now)
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

func TestPilotOperationsManagedFaultDiagnosticsExpireFromEvent(t *testing.T) {
	f, h, provider, router := qualificationFixture(t)
	ctx := context.Background()
	at := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	now := at
	_, ops, notices, _ := pilotOperationsFixture(t, f, h, func() time.Time { return now })
	body := f.Body(t, "/api/qualification-gaps")
	denied := httptest.NewRecorder()
	noSession := httptest.NewRequest("POST", "/api/qualification-gaps", bytes.NewReader(body))
	noSession.Header.Set("Origin", aiadmission.Origin)
	noSession.Header.Set("X-CSRF-Token", "synthetic")
	noSession.AddCookie(&http.Cookie{Name: "__Host-careeros_csrf", Value: "synthetic"})
	router.ServeHTTP(denied, noSession)
	if denied.Code != http.StatusUnauthorized || len(*notices) != 0 {
		t.Fatalf("expected auth denial signalled as a fault: %d", denied.Code)
	}
	h.SetFault("operations_control_read", func() error { return aicontrol.ErrUnavailable })
	fault := qualificationRequest(t, f, router, "/api/qualification-gaps", "control-read-unavailable", "", body)
	if fault.Code < 500 || len(provider.Calls) != 0 || len(*notices) != 1 || (*notices)[0].Kind != "control_fault" {
		t.Fatalf("actual managed control failure did not signal: HTTP %d calls %d notices %+v", fault.Code, len(provider.Calls), *notices)
	}
	h.SetFault("operations_control_read", nil)
	now = at.Add(7*24*time.Hour - time.Second)
	rows, err := ops.OwnerDiagnostics(ctx, f.Principal)
	if err != nil || len(rows) != 1 || rows[0].Kind != accountstore.OperationsControlFault || !rows[0].EventAt.Equal(at) {
		t.Fatalf("content-free diagnostic at last retained second: %+v %v", rows, err)
	}
	now = at.Add(7 * 24 * time.Hour)
	rows, err = ops.OwnerDiagnostics(ctx, f.Principal)
	if err != nil || len(rows) != 0 {
		t.Fatalf("diagnostic visible at seven days: %+v %v", rows, err)
	}
	if err = ops.PruneOrdinaryRecords(ctx, f.Principal); err != nil {
		t.Fatal(err)
	}
	status, err := ops.OwnerStatus(ctx, f.Principal)
	if err != nil || len(status.Incidents) != 1 {
		t.Fatalf("restricted incident shared the diagnostic TTL: %+v %v", status, err)
	}
}

func TestPilotOperationsSaturatedJournalDeniesAcrossIndependentOpens(t *testing.T) {
	f, h, provider, router := qualificationFixture(t)
	ctx := context.Background()
	path := t.TempDir()
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	cfg := accountstore.OperationsJournalConfig{Directory: path, BindingKey: sha256.Sum256([]byte("synthetic-saturated-journal")),
		MaxBytes: 512, Notify: func(context.Context, accountstore.OperationsNotification) accountstore.OperationsDelivery {
			return accountstore.OperationsDelivered
		}}
	j, err := accountstore.OpenLocalOperationsJournal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ops, err := f.Store.LocalOperations(accountstore.OperationsConfig{Owner: "Henrique", Build: "synthetic-build"}, h.LocalOperationsSource(), j)
	if err != nil {
		t.Fatal(err)
	}
	owner := auth.IdentityRef{Provider: "google", Issuer: "https://accounts.google.com", Subject: "fixture-account"}
	if err = f.Store.BootstrapLocalOperationsRole(ctx, owner, "owner"); err != nil {
		t.Fatal(err)
	}
	key := sha256.Sum256([]byte("synthetic-saturated-owner"))
	if err = ops.ProvisionOfflineOwner(ctx, f.Principal, key); err != nil {
		t.Fatal(err)
	}
	if err = j.SignalOfflineFault(ctx, accountstore.OperationsControlFault); !errors.Is(err, auth.ErrUnavailable) {
		t.Fatalf("expected independent journal quota to prevent the required append: %v", err)
	}
	separate, err := accountstore.OpenLocalOperationsJournal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = separate.WithOfflineOwnerStatus(ctx, key, func(s accountstore.OperationsStatus) error {
		if !s.ContainmentUnavailable || len(s.Incidents) != 0 {
			t.Fatalf("old authenticated list was lost or readable as open: %+v", s)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	another, err := accountstore.Open(ctx, f.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer another.Close()
	if _, err = another.LocalOperations(accountstore.OperationsConfig{Owner: "Henrique", Build: "synthetic-build"}, h.LocalOperationsSource(), separate); err != nil {
		t.Fatal(err)
	}
	if _, err = another.PilotCommand(ctx, f.Principal, "deny-before-sql-sync", accountstore.PilotCommand{Kind: "invite", Identity: owner}); err == nil {
		t.Fatal("fresh instance issued an invitation from stale authenticated state")
	}
	res := qualificationRequest(t, f, router, "/api/qualification-gaps", "deny-saturated-journal", "", f.Body(t, "/api/qualification-gaps"))
	if res.Code == 200 || len(provider.Calls) != 0 {
		t.Fatalf("fresh journal allowed protected inference: %d, calls %d", res.Code, len(provider.Calls))
	}
	if err = ops.ObserveFault(ctx, accountstore.OperationsControlFault); !errors.Is(err, auth.ErrUnavailable) {
		t.Fatalf("required write should remain sealed: %v", err)
	}
	controls, err := f.Store.LocalControls(nil).Status(ctx, f.ControlOperator.Principal, "")
	if err != nil || !controls.InvitationStopped || !controls.InferenceStopped {
		t.Fatalf("shared authority fallback: %+v %v", controls, err)
	}
}

func TestPilotOperationsRechecksOwnerAfterIndependentJournalWait(t *testing.T) {
	f, h, _, _ := qualificationFixture(t)
	ctx := context.Background()
	path := t.TempDir()
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	cfg := accountstore.OperationsJournalConfig{Directory: path, BindingKey: sha256.Sum256([]byte("synthetic-expiring-owner")),
		Notify: func(context.Context, accountstore.OperationsNotification) accountstore.OperationsDelivery {
			return accountstore.OperationsDelivered
		}}
	j, err := accountstore.OpenLocalOperationsJournal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	ops, err := f.Store.LocalOperations(accountstore.OperationsConfig{Owner: "Henrique", Build: "synthetic-build",
		VerifyPreparedCover: func(context.Context, accountstore.OperationsAbsence) error { close(started); return auth.ErrForbidden },
	}, h.LocalOperationsSource(), j)
	if err != nil {
		t.Fatal(err)
	}
	owner := auth.IdentityRef{Provider: "google", Issuer: "https://accounts.google.com", Subject: "fixture-account"}
	if err = f.Store.BootstrapLocalOperationsRole(ctx, owner, "owner"); err != nil {
		t.Fatal(err)
	}
	lock, err := os.OpenFile(filepath.Join(path, "journal.lock"), os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	var expiry time.Time
	if err = f.DB.QueryRow(`UPDATE careeros_sessions SET expires_at=clock_timestamp()+interval '1 second'
	  WHERE id=$1 RETURNING expires_at`, f.Principal.SessionID).Scan(&expiry); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() {
		now := time.Now()
		_, err := ops.PlanAbsence(ctx, f.Principal, accountstore.OperationsAbsence{Starts: now.Add(time.Hour), Ends: now.Add(2 * time.Hour),
			NextUpdateAt: now.Add(time.Hour), PreparedCover: true, RightsOwner: true})
		finished <- err
	}()
	select {
	case <-started:
	case e := <-finished:
		t.Fatalf("request failed before waiting for journal: %v", e)
	case <-time.After(5 * time.Second):
		t.Fatal("request did not enter the authorized plan")
	}
	time.Sleep(time.Until(expiry) + 50*time.Millisecond)
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("presence of lock prevented test completion")
	}
	if !errors.Is(err, auth.ErrAuthenticationRequired) {
		t.Fatalf("expired session changed independent containment: %v", err)
	}
	if _, err = j.PublicStatus(ctx); err != accountstore.ErrNotFound {
		t.Fatalf("expired session published a notice: %v", err)
	}
}

func TestPilotOperationsAdmissionJournalLeaseOrdersActionAndRetryCommits(t *testing.T) {
	for _, retry := range []bool{false, true} {
		name := "action"
		if retry {
			name = "retry"
		}
		t.Run(name, func(t *testing.T) {
			f, h, _, router := qualificationFixture(t)
			ctx := context.Background()
			body := f.Body(t, "/api/qualification-gaps")
			var previous *aicontrol.Session
			if retry {
				if err := h.SetQualificationResponder(func(context.Context, string, *http.Request) (*http.Response, bool, error) {
					response := httptest.NewRecorder()
					response.WriteHeader(http.StatusTooManyRequests)
					_, _ = response.WriteString(`{"error":"synthetic_rate_limit"}`)
					return response.Result(), true, nil
				}); err != nil {
					t.Fatal(err)
				}
				got := qualificationRequest(t, f, router, "/api/qualification-gaps", "prior-operations-retry", "", body)
				if got.Code == 200 {
					t.Fatal("expected known failed previous attempt")
				}
				var err error
				previous, err = h.Open(ctx, f.Principal, "prior-operations-retry", "qualification_gaps", body)
				if err != nil {
					t.Fatal(err)
				}
			}
			j, _, _, _ := pilotOperationsFixture(t, f, h, time.Now)
			arrived, finish := make(chan struct{}), make(chan struct{})
			defer func() {
				select {
				case <-finish:
				default:
					close(finish)
				}
			}()
			h.SetFault("before_operations_"+name+"_commit", func() error { close(arrived); <-finish; return nil })
			type outcome struct {
				session *aicontrol.Session
				err     error
			}
			committed := make(chan outcome, 1)
			go func() {
				var s *aicontrol.Session
				var e error
				if retry {
					s, e = previous.RetryQualification(ctx, "operations-retry-commit")
				} else {
					s, e = h.Open(ctx, f.Principal, "operations-action-commit", "qualification_gaps", body)
				}
				committed <- outcome{s, e}
			}()
			select {
			case <-arrived:
			case got := <-committed:
				t.Fatalf("admission never reached commit: %v", got.err)
			case <-time.After(5 * time.Second):
				t.Fatal("admission never reached the actual transaction seam")
			}
			observationStarted := make(chan struct{})
			observed := make(chan error, 1)
			go func() {
				close(observationStarted)
				observed <- j.SignalOfflineFault(ctx, accountstore.OperationsControlFault)
			}()
			<-observationStarted
			select {
			case err := <-observed:
				t.Fatalf("independent stop won before the protected transaction committed: %v", err)
			case <-time.After(100 * time.Millisecond):
			}
			close(finish)
			got := <-committed
			if got.err != nil || got.session == nil {
				t.Fatalf("transaction holding the lease did not commit: %v", got.err)
			}
			if err := <-observed; err != nil {
				t.Fatal(err)
			}
			if retry {
				if got.session.ID() != previous.ID() {
					t.Fatalf("retry changed original Action identity")
				}
				var attempt int
				if err := f.DB.QueryRow(`SELECT attempt FROM ai_fixture_action WHERE id=$1`, got.session.ID()).Scan(&attempt); err != nil || attempt != 2 {
					t.Fatalf("expected durable second attempt: %d %v", attempt, err)
				}
			}
			h.SetFault("before_operations_"+name+"_commit", nil)
			if _, err := h.Open(ctx, f.Principal, "after-independent-stop", "qualification_gaps", body); err == nil {
				t.Fatal("new Action admitted after independent stop")
			}
		})
	}
}

func TestPilotOperationsReplayDeliversLostIntentWithoutInspectOrResend(t *testing.T) {
	f, h, provider, router := qualificationFixture(t)
	_, ops, notices, _ := pilotOperationsFixture(t, f, h, time.Now)
	ctx := context.Background()
	body := f.Body(t, "/api/qualification-gaps")
	h.SetFault("after_intent_commit", func() error { return aicontrol.ErrUnavailable })
	lost := qualificationRequest(t, f, router, "/api/qualification-gaps", "durable-observation-before-exit", "", body)
	if lost.Code < 400 || len(*notices) != 0 || len(provider.Calls) != 0 {
		t.Fatalf("lost reply did not occur in the durable transition gap: %d %+v %v", lost.Code, *notices, provider.Calls)
	}
	h.SetFault("after_intent_commit", nil)
	replay := qualificationRequest(t, f, router, "/api/qualification-gaps", "durable-observation-before-exit", "", body)
	if replay.Code != http.StatusAccepted || len(*notices) != 1 || (*notices)[0].Kind != "unknown_dispatch" || len(provider.Calls) != 0 {
		t.Fatalf("replay failed to deliver exactly one durable observation without another provider entry: %d %+v %v", replay.Code, *notices, provider.Calls)
	}
	status, err := ops.OwnerStatus(ctx, f.Principal)
	if err != nil || len(status.Incidents) != 1 || status.Incidents[0].Exposure != 1344 {
		t.Fatalf("replayed durable original count: %+v %v", status, err)
	}
}

func TestPilotOperationsFailedObservationPreservesProviderFreeActionRecovery(t *testing.T) {
	f, h, provider, router := qualificationFixture(t)
	ctx := context.Background()
	_, _, _, directory := pilotOperationsFixture(t, f, h, time.Now)
	body := f.Body(t, "/api/qualification-gaps")
	h.SetFault("after_intent_commit", func() error { return aicontrol.ErrUnavailable })
	lost := qualificationRequest(t, f, router, "/api/qualification-gaps", "recover-after-observer-loss", "", body)
	if lost.Code < 400 || len(provider.Calls) != 0 {
		t.Fatalf("fixture did not lose the response after durable intent: %d", lost.Code)
	}
	var pending int
	if err := f.DB.QueryRow(`SELECT count(*) FROM careeros_operations_pending`).Scan(&pending); err != nil || pending == 0 {
		t.Fatalf("fixture lacks the committed undelivered observation: %d %v", pending, err)
	}
	h.SetFault("after_intent_commit", nil)
	j, err := accountstore.OpenLocalOperationsJournal(accountstore.OperationsJournalConfig{Directory: directory,
		BindingKey: sha256.Sum256([]byte("synthetic-operations-integration")), MaxBytes: 512})
	if err != nil {
		t.Fatal(err)
	}
	if err = j.SignalOfflineFault(ctx, accountstore.OperationsControlFault); !errors.Is(err, auth.ErrUnavailable) {
		t.Fatalf("fixture public signal should have sealed the unavailable journal: %v", err)
	}
	s, err := h.Open(ctx, f.Principal, "recover-after-observer-loss", "qualification_gaps", body)
	if err != nil || s == nil || s.ID() == "" || len(provider.Calls) != 0 {
		t.Fatalf("provider-free recovery was lost with observation delivery: %v calls=%d", err, len(provider.Calls))
	}
	if _, err = h.Open(ctx, f.Principal, "new-action-after-observer-loss", "qualification_gaps", body); err == nil {
		t.Fatal("failed persistence allowed a new Action")
	}
	var retained int
	if err = f.DB.QueryRow(`SELECT count(*) FROM careeros_operations_pending`).Scan(&retained); err != nil || retained != pending {
		t.Fatalf("failed publication removed the unacknowledged observations: %d vs %d %v", retained, pending, err)
	}
}

func pilotOperationsFixture(t *testing.T, f *aiadmission.Fixture, h *aicontrol.Harness, now func() time.Time) (*accountstore.OperationsJournal, *accountstore.Operations, *[]accountstore.OperationsNotification, string) {
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
	return j, ops, &notices, directory
}

func TestPilotOperationsWarningAbsenceAndVerifiedReopening(t *testing.T) {
	f, h, _, router := qualificationFixture(t)
	ctx := context.Background()
	now := time.Now()
	j, ops, _, directory := pilotOperationsFixture(t, f, h, func() time.Time { return now })
	var checkedIncident string
	var verifiedBackupAt time.Time
	ops, err := f.Store.LocalOperations(accountstore.OperationsConfig{Owner: "Henrique", Build: "synthetic-build",
		VerifyBackupRecovery: func(_ context.Context, reference string, incident accountstore.OperationsIncident) (time.Time, error) {
			if reference != "fixture-verified-backup" || incident.ID != checkedIncident || verifiedBackupAt.IsZero() {
				return time.Time{}, auth.ErrForbidden
			}
			return verifiedBackupAt, nil
		},
	}, h.LocalOperationsSource(), j)
	if err != nil {
		t.Fatal(err)
	}
	body := f.Body(t, "/api/qualification-gaps")
	w := qualificationRequest(t, f, router, "/api/qualification-gaps", "operations-review", "", body)
	if w.Code != 200 {
		t.Fatalf("qualify before warning: %d", w.Code)
	}
	if err := ops.Inspect(ctx); err != nil {
		t.Fatal(err)
	}
	status, err := ops.OwnerStatus(ctx, f.Principal)
	if err != nil || len(status.Incidents) != 4 {
		t.Fatalf("warning: %+v %v", status, err)
	}
	var warn accountstore.OperationsIncident
	for _, incident := range status.Incidents {
		if incident.Kind == "spend_warning" {
			warn = incident
			continue
		}
		if err = ops.ReviewIncident(ctx, f.Principal, incident.ID, incident.Revision, false); err != nil {
			t.Fatal(err)
		}
	}
	if warn.ID == "" {
		t.Fatal("expected a spend warning")
	}
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
			if err = ops.ReviewIncident(ctx, f.Principal, incident.ID, incident.Revision, true); !errors.Is(err, auth.ErrForbidden) {
				t.Fatalf("cause-blind backup resolution: %v", err)
			}
			checkedIncident = incident.ID
			now = now.Add(time.Minute)
			verifiedBackupAt = now
			if err = ops.ResolveIncident(ctx, f.Principal, incident.ID, incident.Revision, "fixture-verified-backup"); err != nil {
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
	unrelated := qualificationParticipant(t, f, "unrelated-operations-compromise")
	current, err := controls.Status(ctx, operator, "")
	if err != nil {
		t.Fatal(err)
	}
	stoppedUnrelated, err := controls.Apply(ctx, operator, "stop-unrelated-for-status", accountstore.ControlCommand{
		Control: "compromise", Stop: true, ExpectedVersion: current.Version, AccountID: unrelated.Principal.AccountID})
	if err != nil {
		t.Fatal(err)
	}
	unrelatedReopen := accountstore.ControlCommand{Control: "compromise", ExpectedVersion: stoppedUnrelated.Version, AccountID: unrelated.Principal.AccountID}
	unrelatedReopen.RecoveryReference, err = verified.Prepare(ctx, operator, unrelatedReopen)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.Store.LocalControls(verified).Apply(ctx, operator, "resolve-unrelated-for-status", unrelatedReopen); err != nil {
		t.Fatal(err)
	}
	if err = ops.PublishVerifiedRecovery(ctx, f.Principal, unrelatedReopen.RecoveryReference, "limited_pilot"); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("unrelated current compromise proof closed public outage: %v", err)
	}
	assertPublicClosureSessionExpires(t, f, ops, directory, lastReference)
	renewed := controlFreshIdentity(t, f.Service, "fixture-account")
	f.Principal = renewed.Principal
	if err = ops.PublishVerifiedRecovery(ctx, f.Principal, lastReference, "limited_pilot"); err != nil {
		t.Fatal(err)
	}
	closed, err := j.PublicStatus(ctx)
	if err != nil || closed.Status != "recovered" || closed.NextUpdateAt != nil || closed.Limitation != "limited_pilot" {
		t.Fatalf("verified closure: %+v %v", closed, err)
	}
	if err = j.PublishOutage(ctx, "invitations_and_inference", "investigating", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err = ops.PublishVerifiedRecovery(ctx, f.Principal, lastReference, "limited_pilot"); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("verification predating a new episode still permitted public closure: %v", err)
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
	if err = ops.PublishVerifiedRecovery(ctx, f.Principal, lastReference, "limited_pilot"); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("previous episode reference remained valid: %v", err)
	}
	now = plan.Starts.Add(time.Hour)
	status, err = ops.OwnerStatus(ctx, f.Principal)
	if err != nil {
		t.Fatal(err)
	}
	for _, incident := range status.Incidents {
		if incident.Kind != "operator_absence" {
			continue
		}
		if err = ops.ReviewIncident(ctx, f.Principal, incident.ID, incident.Revision, true); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("uncovered absence reopened inside the interval: %v", err)
		}
		now = plan.Ends
		if err = ops.ReviewIncident(ctx, f.Principal, incident.ID, incident.Revision, true); err != nil {
			t.Fatal(err)
		}
	}
}

func assertPublicClosureSessionExpires(t *testing.T, f *aiadmission.Fixture, ops *accountstore.Operations, directory, reference string) {
	t.Helper()
	lock, err := os.OpenFile(filepath.Join(directory, "status.lock"), os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	var expiry time.Time
	if err = f.DB.QueryRow(`UPDATE careeros_sessions SET expires_at=clock_timestamp()+interval '2 seconds'
	  WHERE id=$1 RETURNING expires_at`, f.Principal.SessionID).Scan(&expiry); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	principal := f.Principal
	done := make(chan error, 1)
	go func() { done <- ops.PublishVerifiedRecovery(ctx, principal, reference, "limited_pilot") }()
	for {
		_, err = f.DB.ExecContext(ctx, `SELECT id FROM careeros_accounts WHERE id=$1 FOR UPDATE NOWAIT`, principal.AccountID)
		var code interface{ SQLState() string }
		if errors.As(err, &code) && code.SQLState() == "55P03" {
			break
		}
		if err != nil || !time.Now().Before(expiry) {
			t.Fatalf("owner transaction never began before expiry: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(time.Until(expiry) + 50*time.Millisecond)
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
	case <-ctx.Done():
		t.Fatal("expired publication did not exit the test lock")
	}
	if !errors.Is(err, auth.ErrAuthenticationRequired) {
		t.Fatalf("expired owner could finish public closure: %v", err)
	}
}
