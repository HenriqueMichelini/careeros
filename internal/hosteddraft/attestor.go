package hosteddraft

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aicontrol"
)

const (
	fakeNamespace   = "careeros_294_draft"
	attestationPath = "/internal/draft-attestations/v1"
	maxAttestation  = 1024
	proofPrefix     = "CareerOS-Draft-SHA256 "
)

var (
	errInvalidAttestation     = errors.New("hosted fake Draft attestation invalid")
	errAttestationUnavailable = errors.New("hosted fake Draft attestation unavailable")
	attestationID             = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
	attestationHost           = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)*$`)
)

type attestation struct {
	Fixture       string `json:"fixture"`
	Namespace     string `json:"namespace"`
	DispatchID    string `json:"dispatchId"`
	ActionID      string `json:"actionId"`
	Stage         string `json:"stage"`
	Provider      string `json:"provider"`
	RequestDigest string `json:"requestDigest"`
	ResultDigest  string `json:"resultDigest"`
	HandoffNonce  string `json:"handoffNonce"`
}

func dispatchAttestation(call aicontrol.HostedDraftCall, result [32]byte) attestation {
	return attestation{
		Fixture: FixtureVersion, Namespace: call.Namespace, DispatchID: call.DispatchID,
		ActionID: call.ActionID, Stage: call.Stage, Provider: call.Provider,
		RequestDigest: hex.EncodeToString(call.RequestDigest[:]), ResultDigest: hex.EncodeToString(result[:]),
		HandoffNonce: hex.EncodeToString(call.HandoffNonce[:]),
	}
}

type attestationBindings struct {
	request, result, nonce [sha256.Size]byte
}

func (a attestation) bindings() (attestationBindings, bool) {
	var bindings attestationBindings
	if a.Fixture != FixtureVersion || a.Namespace != fakeNamespace || !attestationID.MatchString(a.DispatchID) ||
		!attestationID.MatchString(a.ActionID) || stageProvider(a.Stage) != a.Provider || a.Provider == "" {
		return bindings, false
	}
	if !decodeProofDigest(a.RequestDigest, &bindings.request) || !decodeProofDigest(a.ResultDigest, &bindings.result) ||
		!decodeProofDigest(a.HandoffNonce, &bindings.nonce) {
		return bindings, false
	}
	body, err := fixtureResponse(a.Stage)
	if err != nil {
		return bindings, false
	}
	expected := sha256.Sum256(body)
	return bindings, subtle.ConstantTimeCompare(bindings.result[:], expected[:]) == 1
}

func decodeProofDigest(text string, digest *[sha256.Size]byte) bool {
	if len(text) != sha256.Size*2 {
		return false
	}
	_, err := hex.Decode(digest[:], []byte(text))
	return err == nil && text == hex.EncodeToString(digest[:]) && *digest != ([sha256.Size]byte{})
}

// AttestorClient holds the fixed independent service address and app request
// credential. It cannot write the attestation database or forward prompts.
type AttestorClient struct {
	endpoint string
	key      [32]byte
	identity func(context.Context) (string, error)
	client   *http.Client
}

// NewAttestorClient accepts one Cloud Run HTTPS origin without a redirect or
// proxy path. The platform token is independent of the application credential.
func NewAttestorClient(origin string, token [32]byte, identity func(context.Context) (string, error)) (*AttestorClient, error) {
	return newAttestorClient(origin, token, identity, false)
}

func newAttestorClient(origin string, token [32]byte, identity func(context.Context) (string, error), fixture bool) (*AttestorClient, error) {
	endpoint, err := url.Parse(origin)
	if err != nil || endpoint.Scheme != "https" || !attestationHost.MatchString(endpoint.Hostname()) ||
		len(endpoint.Hostname()) > 253 || net.ParseIP(endpoint.Hostname()) != nil || endpoint.User != nil ||
		origin != "https://"+endpoint.Host || token == ([32]byte{}) {
		return nil, errInvalidAttestation
	}
	if !fixture && (!strings.HasSuffix(endpoint.Hostname(), ".run.app") || identity == nil) {
		return nil, errInvalidAttestation
	}
	if endpoint.Hostname() == "localhost" {
		port, parseErr := strconv.ParseUint(endpoint.Port(), 10, 16)
		if parseErr != nil || port < 1024 || endpoint.Host != "localhost:"+strconv.FormatUint(port, 10) {
			return nil, errInvalidAttestation
		}
	} else if endpoint.Host != endpoint.Hostname() || !strings.Contains(endpoint.Host, ".") {
		return nil, errInvalidAttestation
	}
	transport := &http.Transport{
		DialContext:            (&net.Dialer{Timeout: 2 * time.Second}).DialContext,
		TLSClientConfig:        &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:    2 * time.Second,
		ResponseHeaderTimeout:  3 * time.Second,
		MaxResponseHeaderBytes: 4096,
		MaxConnsPerHost:        2,
		MaxIdleConnsPerHost:    1,
		IdleConnTimeout:        15 * time.Second,
		DisableCompression:     true,
	}
	return &AttestorClient{
		endpoint: origin + attestationPath, key: token, identity: identity,
		client: &http.Client{
			Transport: transport, Timeout: 5 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

// NewAttestorClientWithHTTPClient accepts the explicit transport of a private
// TLS fixture. Origin checks, deadlines and redirect refusal remain identical.
// The hosted runtime uses NewAttestorClient and its code-owned transport.
func NewAttestorClientWithHTTPClient(origin string, token [32]byte, identity func(context.Context) (string, error), client *http.Client) (*AttestorClient, error) {
	if client == nil || client.Transport == nil {
		return nil, errInvalidAttestation
	}
	attestor, err := newAttestorClient(origin, token, identity, true)
	if err != nil {
		return nil, err
	}
	attestor.client.Transport = client.Transport
	return attestor, nil
}

func (c *AttestorClient) record(ctx context.Context, evidence attestation) error {
	if _, valid := evidence.bindings(); !valid {
		return errInvalidAttestation
	}
	body, err := json.Marshal(evidence)
	if err != nil || len(body) > maxAttestation {
		return errInvalidAttestation
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return errAttestationUnavailable
	}
	// This call is nonreplayable. Disable automatic transport replays and do
	// not set an idempotency header even though the request body is buffered.
	request.GetBody = nil
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", attestationAuthorization(c.key, body))
	if c.identity != nil {
		identity, err := c.identity(ctx)
		if err != nil || len(identity) == 0 || len(identity) > 8192 || strings.ContainsAny(identity, " \t\r\n") {
			return errAttestationUnavailable
		}
		request.Header.Set("X-Serverless-Authorization", "Bearer "+identity)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return errAttestationUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return errAttestationUnavailable
	}
	return nil
}

func attestationAuthorization(key [32]byte, body []byte) string {
	proof := hmac.New(sha256.New, key[:])
	_, _ = proof.Write([]byte("POST " + attestationPath + "\n"))
	_, _ = proof.Write(body)
	return proofPrefix + hex.EncodeToString(proof.Sum(nil))
}

// NewAttestorHandler performs independent result-set checking and persists
// metadata with the dedicated SQL role. Callers never supply a credential or
// a database operation through the request body.
func NewAttestorHandler(db *sql.DB, token [32]byte) (http.Handler, error) {
	if db == nil {
		return nil, errInvalidAttestation
	}
	return attestorHandler(token, func(ctx context.Context, a attestation, bindings attestationBindings) (bool, error) {
		var recorded bool
		err := db.QueryRowContext(ctx, `SELECT careeros_294_draft.record_canonical_fake_attestation($1,$2,$3,$4,$5,$6,$7,$8)`,
			a.Namespace, a.DispatchID, a.ActionID, a.Stage, a.Provider, bindings.request[:], bindings.result[:], bindings.nonce[:]).Scan(&recorded)
		return recorded, err
	})
}

func attestorHandler(token [32]byte, record func(context.Context, attestation, attestationBindings) (bool, error)) (http.Handler, error) {
	if token == ([32]byte{}) || record == nil {
		return nil, errInvalidAttestation
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.URL.Path != attestationPath || r.URL.RawQuery != "" || r.URL.RawPath != "" || r.URL.ForceQuery {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		credentials := r.Header.Values("Authorization")
		if len(credentials) != 1 || len(credentials[0]) != len(proofPrefix)+sha256.Size*2 || !strings.HasPrefix(credentials[0], proofPrefix) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Content-Type") != "application/json" || len(r.Header.Values("Content-Type")) != 1 || r.Header.Get("Content-Encoding") != "" {
			w.WriteHeader(http.StatusUnsupportedMediaType)
			return
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxAttestation))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if !hmac.Equal([]byte(credentials[0]), []byte(attestationAuthorization(token, raw))) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var evidence attestation
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if !accountstore.StrictJSON(raw) || decoder.Decode(&evidence) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		bindings, valid := evidence.bindings()
		if !valid {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		accepted, err := record(ctx, evidence, bindings)
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if !accepted {
			w.WriteHeader(http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}), nil
}
