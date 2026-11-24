package accountstore

import (
	"crypto/tls"
	"errors"
	"strings"
	"testing"

	"professional-information-repo/internal/auth"
)

func TestHostedPilotConfigPinsVerifiedDirectTLS(t *testing.T) {
	t.Setenv("PGSERVICE", "")
	t.Setenv("PGOPTIONS", "-c search_path=public")
	t.Setenv("PGHOST", "ambient.example.invalid")
	t.Setenv("PGPORT", "32100")
	dsn := "postgresql://careeros_294_runtime:synthetic-password@localhost:32124/careeros_294?sslmode=verify-full"
	config, err := hostedPilotConfig(dsn, hostedPilotRuntime)
	if err != nil {
		t.Fatal(err)
	}
	if config.TLSConfig == nil || config.TLSConfig.InsecureSkipVerify || config.TLSConfig.ServerName != "localhost" ||
		config.TLSConfig.MinVersion != tls.VersionTLS12 || config.Host != "localhost" || config.Port != 32124 ||
		config.Database != "careeros_294" || config.User != "careeros_294_runtime" || len(config.Fallbacks) != 0 ||
		config.RuntimeParams["search_path"] != "careeros_294,pg_catalog" || config.RuntimeParams["options"] != "" {
		t.Fatal("hosted PostgreSQL connection is not pinned to the approved role, target and verified TLS")
	}
	defaultPort, err := hostedPilotConfig(strings.Replace(dsn, ":32124", "", 1), hostedPilotRuntime)
	if err != nil || defaultPort.Port != 5432 {
		t.Fatalf("omitted port did not select 5432: %v", err)
	}
	invalid := []string{
		strings.Replace(dsn, "verify-full", "require", 1),
		dsn + "&host=another.example.invalid",
		strings.Replace(dsn, "localhost", "ep-candidate-pooler.sa-east-1.aws.neon.tech", 1),
		strings.Replace(dsn, "localhost", "localhost,other.invalid", 1),
		strings.Replace(dsn, "localhost", "127.0.0.1", 1),
		strings.Replace(dsn, "careeros_294_runtime", "careeros_294_migrator", 1),
		strings.Replace(dsn, ":synthetic-password", "", 1),
		strings.Replace(dsn, "/careeros_294?", "/postgres?", 1),
	}
	for i, candidate := range invalid {
		if _, err := hostedPilotConfig(candidate, hostedPilotRuntime); !errors.Is(err, auth.ErrUnavailable) {
			t.Errorf("case %d: invalid DSN returned %v", i, err)
		}
	}
	t.Setenv("PGSERVICE", "ambient")
	if _, err := hostedPilotConfig(dsn, hostedPilotRuntime); !errors.Is(err, auth.ErrUnavailable) {
		t.Errorf("ambient service returned %v", err)
	}
}
