package profileingestion

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"time"

	"professional-information-repo/internal/preprocessing"
)

const (
	stageTimeout      = 27 * time.Second
	maxStageBody      = 320 << 10
	maxReceiptPayload = 128 << 10
	receiptTTL        = 10 * time.Minute
	extractionVersion = "profile-claims-prompt-v6/schema-v2/source-resolution-v1/portions-v1/clarifications-v1/apply-answer-v1"
	comparisonVersion = "profile-operations-prompt-v5/schema-v3/dispositions-v1/clarifications-v1/apply-answer-v1"
	receiptVersion    = "profile-ingestion-stages-v1/" + extractionVersion + "/" + comparisonVersion + "/" + preprocessing.RulesVersion + "/" + preprocessing.StructureVersion + "/" + preprocessing.PlanningVersion
)

type stageRequest struct {
	Version int    `json:"version"`
	Kind    string `json:"kind"`
}

// This authenticated payload remains client-held. It is not an authorization to
// save, bypass field validation, or treat an extracted claim as semantic truth.
type extractionReceipt struct {
	Version               string         `json:"version"`
	Binding               string         `json:"binding"`
	ExpiresAt             int64          `json:"expiresAt"`
	Claims                []claim        `json:"claims"`
	UnverifiedCount       int            `json:"unverifiedCount"`
	Skipped               []skippedClaim `json:"skipped"`
	ComparisonInstruction string         `json:"comparisonInstruction"`
}

func validStage(in request) bool {
	if in.Stage == nil {
		return in.Receipt == ""
	}
	if in.Stage.Version != 1 {
		return false
	}
	switch in.Stage.Kind {
	case "extract":
		return in.Receipt == ""
	case "reconcile":
		return in.Receipt != ""
	default:
		return false
	}
}

func requestBinding(in request) string {
	in.Stage, in.Receipt = nil, ""
	// Canonicalize object keys (including RawMessage document keys), preserving
	// numbers and array order. JSON property order is not Profile identity.
	return canonicalBinding(in)
}

func receiptMAC(key, payload []byte) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("careeros/profile-ingestion/receipt/v1\x00"))
	_, _ = mac.Write(payload)
	return mac.Sum(nil)
}

func (a app) issueReceipt(in request, claims []claim, invalid int, ledger *reconciliationContext) (string, int64, string) {
	expires := time.Now().Add(receiptTTL).UnixMilli()
	payload := mustJSON(extractionReceipt{receiptVersion, requestBinding(in), expires, claims, invalid, ledger.skipped, ledger.comparisonInstruction})
	if len(payload) > maxReceiptPayload {
		return "", 0, "capacity"
	}
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(receiptMAC(a.signingKey, payload)), expires, ""
}

func (a app) verifyReceipt(in request) (*extractionReceipt, string) {
	if len(in.Receipt) > base64.RawURLEncoding.EncodedLen(maxReceiptPayload)+1+43 {
		return nil, "capacity"
	}
	parts := strings.Split(in.Receipt, ".")
	if len(parts) != 2 {
		return nil, "receipt_invalid"
	}
	payload, err := base64.RawURLEncoding.Strict().DecodeString(parts[0])
	if err != nil || len(payload) > maxReceiptPayload {
		return nil, "receipt_invalid"
	}
	signature, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if err != nil || !hmac.Equal(signature, receiptMAC(a.signingKey, payload)) {
		return nil, "receipt_invalid"
	}
	var receipt extractionReceipt
	if !strict(payload, &receipt) || receipt.Version != receiptVersion || receipt.Binding != requestBinding(in) {
		return nil, "receipt_invalid"
	}
	now := time.Now().UnixMilli()
	if receipt.ExpiresAt <= now {
		return nil, "receipt_expired"
	}
	return &receipt, ""
}
