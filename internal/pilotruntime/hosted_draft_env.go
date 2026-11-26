package pilotruntime

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"professional-information-repo/internal/auth"
)

// HostedDraftSecret treats configuration errors as unavailable without
// writing the value to logs.
func HostedDraftSecret(value string) ([32]byte, error) {
	var key [32]byte
	raw, err := base64.StdEncoding.Strict().DecodeString(value)
	if err != nil || len(raw) != len(key) || base64.StdEncoding.EncodeToString(raw) != value {
		return key, auth.ErrUnavailable
	}
	copy(key[:], raw)
	if key == ([32]byte{}) {
		return key, auth.ErrUnavailable
	}
	return key, nil
}

// CloudRunIdentityToken obtains a token for the exact attestor audience from
// the runtime metadata service. The endpoint ignores ambient proxy settings.
func CloudRunIdentityToken(audience string) func(context.Context) (string, error) {
	const metadata = "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/identity"
	target := metadata + "?" + url.Values{"audience": {audience}, "format": {"full"}}.Encode()
	client := &http.Client{
		Transport: &http.Transport{DisableKeepAlives: true},
		Timeout:   2 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return func(ctx context.Context) (string, error) {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return "", auth.ErrUnavailable
		}
		request.Header.Set("Metadata-Flavor", "Google")
		response, err := client.Do(request)
		if err != nil {
			return "", auth.ErrUnavailable
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK || response.Header.Get("Metadata-Flavor") != "Google" {
			return "", auth.ErrUnavailable
		}
		raw, err := io.ReadAll(io.LimitReader(response.Body, 8193))
		token := string(raw)
		if err != nil || len(raw) > 8192 || strings.Count(token, ".") != 2 || strings.ContainsAny(token, " \t\r\n") {
			return "", auth.ErrUnavailable
		}
		return token, nil
	}
}
