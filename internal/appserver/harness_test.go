package appserver

import (
	"errors"
	"os"
	"testing"

	"harness/internal/conversations"
)

func TestMethodErrorsDoNotConfuseMissingFilesWithMissingSession(t *testing.T) {
	if mapped := methodError(os.ErrNotExist); mapped != os.ErrNotExist {
		t.Fatal("storage failure was reclassified as missing session")
	}
	assertMethodError(t, methodError(conversations.ErrSessionNotFound), CodeNotFound)
	assertMethodError(t, methodError(conversations.ErrWorkspace), CodeInvalidParams)
	assertMethodError(t, methodError(conversations.ErrInvalidRunSettings), CodeInvalidParams)
	assertMethodError(t, methodError(conversations.ErrInvalidCommand), CodeInvalidParams)
	assertMethodError(t, methodError(conversations.ErrCommandRejected), CodeConflict)
}

func assertMethodError(t *testing.T, err error, code ErrorCode) {
	t.Helper()
	var public *Error
	if !errors.As(err, &public) || public.Code != code {
		t.Fatalf("error=%v, want %s", err, code)
	}
}
