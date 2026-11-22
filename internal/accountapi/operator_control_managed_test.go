package accountapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aicontrol"
)

func TestOperatorControlInferenceStopUsesSharedAdmission(t *testing.T) {
	f, h, provider, router := qualificationFixture(t)
	ctx := context.Background()
	controls := f.Store.LocalControls(nil)
	if _, err := controls.Apply(ctx, f.ControlOperator.Principal, "stop-new-inference", accountstore.ControlCommand{Control: "inference", Stop: true, ExpectedVersion: 1}); err != nil {
		t.Fatal(err)
	}
	body := f.Body(t, "/api/qualification-gaps")
	if _, err := h.Open(ctx, f.Principal, "raw-core-after-stop", "qualification_gaps", body); err != aicontrol.ErrDenied {
		t.Fatalf("raw shared admission after authoritative stop: %v", err)
	}
	for _, path := range []string{"/api/qualification-gaps", "/.netlify/functions/qualification-gaps", "/.netlify/functions/account/api/qualification-gaps"} {
		response := qualificationRequest(t, f, router, path, "public-after-stop", "", body)
		if response.Code < 400 || len(provider.Calls) != 0 {
			t.Fatalf("stopped managed alias %s: %d calls=%d", path, response.Code, len(provider.Calls))
		}
	}
	status, err := f.Store.PilotStatus(ctx, f.Principal)
	if err != nil || !status.CanRetrieve || !status.CanStandalone {
		t.Fatalf("inference stop cannot revoke retained rights: %+v %v", status, err)
	}
}

func TestOperatorControlInferenceStopDeniesLaterLegButAllowsCompletePublication(t *testing.T) {
	for _, stopAfter := range []int32{1, 3} {
		t.Run(map[int32]string{1: "later_leg", 3: "complete_publication"}[stopAfter], func(t *testing.T) {
			f, h, provider, router := qualificationFixture(t)
			var finished atomic.Int32
			h.SetFault("after_controlled_callback", func() error {
				if finished.Add(1) == stopAfter {
					_, err := f.Store.LocalControls(nil).Apply(context.Background(), f.ControlOperator.Principal, "stop-after-reached-callback", accountstore.ControlCommand{Control: "inference", Stop: true, ExpectedVersion: 1})
					return err
				}
				return nil
			})
			response := qualificationRequest(t, f, router, "/api/qualification-gaps", "separate-publication-right", "", f.Body(t, "/api/qualification-gaps"))
			if len(provider.Calls) != int(stopAfter) {
				t.Fatalf("stop callback count: %d", len(provider.Calls))
			}
			if stopAfter == 1 && response.Code < 400 || stopAfter == 3 && response.Code != http.StatusOK {
				t.Fatalf("separate inference/publication: %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestOperatorControlCallbackFirstOrdersBeforeIndependentStop(t *testing.T) {
	f, h, provider, router := qualificationFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once atomic.Bool
	if err := h.SetQualificationResponder(func(ctx context.Context, providerName string, request *http.Request) (*http.Response, bool, error) {
		if once.CompareAndSwap(false, true) {
			close(entered)
			<-release
		}
		response, err := provider.Respond(ctx, providerName, request)
		return response, true, err
	}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	body := f.Body(t, "/api/qualification-gaps")
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- qualificationRequest(t, f, router, "/api/qualification-gaps", "callback-before-stop", "", body)
	}()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("positive callback never entered")
	}
	u, _ := url.Parse(f.DSN)
	query := u.Query()
	query.Set("application_name", "operator_stop_other_instance")
	u.RawQuery = query.Encode()
	other, err := accountstore.Open(context.Background(), u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	stopped := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err := other.LocalControls(nil).Apply(ctx, f.ControlOperator.Principal, "independent-stop", accountstore.ControlCommand{Control: "inference", Stop: true, ExpectedVersion: 1})
		stopped <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting bool
		if err = f.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE application_name='operator_stop_other_instance' AND wait_event_type='Lock')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("independent stop never waited on the actual entered callback")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case err := <-stopped:
		t.Fatalf("stop claimed completion before callback returned: %v", err)
	default:
	}
	close(release)
	if err = <-stopped; err != nil {
		t.Fatal(err)
	}
	response := <-done
	if len(provider.Calls) != 1 || response.Code < 400 {
		t.Fatalf("later leg after winning stop: %d calls=%d", response.Code, len(provider.Calls))
	}
}

func TestOperatorControlQueuedStopCannotDeadlockLeaseValidation(t *testing.T) {
	f, h, provider, router := qualificationFixture(t)
	leased, resume := make(chan struct{}), make(chan struct{})
	var once atomic.Bool
	h.SetFault("after_dispatch_lease", func() error {
		if once.CompareAndSwap(false, true) {
			close(leased)
			<-resume
		}
		return nil
	})
	defer func() {
		select {
		case <-resume:
		default:
			close(resume)
		}
	}()
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()
	r := f.Request(t, requestCtx, "/api/qualification-gaps", f.Body(t, "/api/qualification-gaps"))
	r.Header.Del("X-OpenAI-Api-Key")
	r.Header.Del("X-TypeSafe-Api-Key")
	r.Header.Set("X-CareerOS-Action-Key", "lease-before-queued-stop")
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { w := httptest.NewRecorder(); router.ServeHTTP(w, r); done <- w }()
	select {
	case <-leased:
	case <-time.After(10 * time.Second):
		t.Fatal("valid request never acquired dispatch lease")
	}
	u, _ := url.Parse(f.DSN)
	q := u.Query()
	q.Set("application_name", "operator_stop_after_lease")
	u.RawQuery = q.Encode()
	other, err := accountstore.Open(context.Background(), u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	stopCtx, cancelStop := context.WithCancel(context.Background())
	defer cancelStop()
	stopped := make(chan error, 1)
	go func() {
		_, err := other.LocalControls(nil).Apply(stopCtx, f.ControlOperator.Principal, "queued-stop-after-lease", accountstore.ControlCommand{Control: "inference", Stop: true, ExpectedVersion: 1})
		stopped <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting bool
		if err = f.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE application_name='operator_stop_after_lease' AND wait_event_type='Lock')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("operator stop did not queue behind dispatch lease")
		}
		time.Sleep(time.Millisecond)
	}
	close(resume)
	select {
	case err = <-stopped:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		cancelRequest()
		cancelStop()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
		t.Fatal("queued operator and leased validation formed a two-connection cycle")
	}
	response := <-done
	if len(provider.Calls) != 1 || response.Code < 400 {
		t.Fatalf("callback orders before queued stop, later legs denied: %d calls=%d", response.Code, len(provider.Calls))
	}
}

func TestOperatorControlOldIntentCannotSendAfterWinningStop(t *testing.T) {
	f, h, provider, router := qualificationFixture(t)
	checked, release := make(chan struct{}), make(chan struct{})
	var once atomic.Bool
	h.SetFault("after_handoff_commit", func() error {
		if once.CompareAndSwap(false, true) {
			close(checked)
			<-release
		}
		return nil
	})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	done := make(chan *httptest.ResponseRecorder, 1)
	body := f.Body(t, "/api/qualification-gaps")
	go func() {
		done <- qualificationRequest(t, f, router, "/api/qualification-gaps", "old-intent-stop", "", body)
	}()
	select {
	case <-checked:
	case <-time.After(10 * time.Second):
		t.Fatal("valid request did not reach the final fake-handoff boundary")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := f.Store.LocalControls(nil).Apply(ctx, f.ControlOperator.Principal, "stop-before-fake-entry", accountstore.ControlCommand{Control: "inference", Stop: true, ExpectedVersion: 1}); err != nil {
		t.Fatal(err)
	}
	close(release)
	response := <-done
	if len(provider.Calls) != 0 || response.Code < 400 {
		t.Fatalf("old intent crossed winning stop: status=%d calls=%d", response.Code, len(provider.Calls))
	}
	var counted int64
	var outstanding int
	if err := f.DB.QueryRow(`SELECT COALESCE(sum(counted),0),count(*) FILTER(WHERE remote_state<>'terminal_proven') FROM ai_fixture_dispatch`).Scan(&counted, &outstanding); err != nil {
		t.Fatal(err)
	}
	if counted != 1344 || outstanding != 1 {
		t.Fatalf("stop must retain unknown durable intent: counted=%d occupancy=%d", counted, outstanding)
	}
}
