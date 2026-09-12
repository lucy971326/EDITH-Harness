package appserver

import (
	"errors"
	"os"
	"testing"

	"harness/products/harness"
)

func TestMethodErrorsDoNotConfuseMissingFilesWithMissingSession(t *testing.T) {
	if mapped := methodError(os.ErrNotExist); mapped != os.ErrNotExist {
		t.Fatal("storage failure was reclassified as missing session")
	}
	assertMethodError(t, methodError(harness.ErrSessionNotFound), CodeNotFound)
	assertMethodError(t, methodError(harness.ErrWorkspace), CodeInvalidParams)
	assertMethodError(t, methodError(harness.ErrInvalidRunSettings), CodeInvalidParams)
	assertMethodError(t, methodError(harness.ErrInvalidCommand), CodeInvalidParams)
	assertMethodError(t, methodError(harness.ErrCommandRejected), CodeConflict)
}

func assertMethodError(t *testing.T, err error, code ErrorCode) {
	t.Helper()
	var public *Error
	if !errors.As(err, &public) || public.Code != code {
		t.Fatalf("error=%v, want %s", err, code)
	}
}
