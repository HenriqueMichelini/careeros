package aicontrol

import (
	"bytes"
	"context"
	"testing"
)

func TestLocalExecutionUpgradeKeepsInheritedRowsUntracked(t *testing.T) {
	f := setup(t, nil)
	s := f.open(t, "before-local-tracking")
	var binding, execution []byte
	if err := f.db.QueryRow(`SELECT binding,execution_binding FROM ai_fixture_action WHERE namespace=$1 AND id=$2`, f.cfg.Namespace, s.ID()).Scan(&binding, &execution); err != nil {
		t.Fatal(err)
	}
	if err := f.h.Close(); err != nil {
		t.Fatal(err)
	}
	// Recreate the actual pre-upgrade column/trigger shape while preserving the
	// original serialized Action identity and installed restricted roles.
	if _, err := f.db.Exec(`DROP TRIGGER ai_fixture_local_execution_guard ON ai_fixture_stage; DROP FUNCTION ai_fixture_local_execution_guard(); ALTER TABLE ai_fixture_action DROP COLUMN local_execution_tracking`); err != nil {
		t.Fatal(err)
	}
	h, err := NewSynthetic(context.Background(), f.dsn, f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	var tracked bool
	var afterBinding, afterExecution []byte
	if err = f.db.QueryRow(`SELECT local_execution_tracking,binding,execution_binding FROM ai_fixture_action WHERE namespace=$1 AND id=$2`, f.cfg.Namespace, s.ID()).Scan(&tracked, &afterBinding, &afterExecution); err != nil || tracked || !bytes.Equal(binding, afterBinding) || !bytes.Equal(execution, afterExecution) {
		t.Fatalf("upgrade fabricated callback evidence or changed identity: tracked=%t err=%v", tracked, err)
	}
	newAction, err := h.Open(context.Background(), f.p, "after-local-tracking", "cv_generation", []byte(`{"synthetic":"root"}`))
	if err != nil {
		t.Fatal(err)
	}
	if err = f.db.QueryRow(`SELECT local_execution_tracking FROM ai_fixture_action WHERE namespace=$1 AND id=$2`, f.cfg.Namespace, newAction.ID()).Scan(&tracked); err != nil || !tracked {
		t.Fatalf("new admission lacks database-owned tracking: %t %v", tracked, err)
	}
}
