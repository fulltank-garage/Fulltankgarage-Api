package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestLineWebhookSignatureValidation(t *testing.T) {
	handler := NewLineWebhookHandler(nil, "line-secret")
	body := []byte(`{"events":[]}`)

	mac := hmac.New(sha256.New, []byte("line-secret"))
	_, _ = mac.Write(body)
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	if !handler.validSignature(signature, body) {
		t.Fatal("expected signature to be valid")
	}

	if handler.validSignature(signature, []byte(`{"events":[{}]}`)) {
		t.Fatal("expected changed body to be invalid")
	}
}

func TestLineWebhookSignatureValidationRejectsMissingSecret(t *testing.T) {
	handler := NewLineWebhookHandler(nil, "")

	if handler.validSignature("signature", []byte(`{"events":[]}`)) {
		t.Fatal("expected missing channel secret to reject signature")
	}
}
