package tui

import (
	"testing"
)

func TestE2E_LoginCommand_SetsExitAction(t *testing.T) {
	m, _ := testModel(t)

	result, _ := submitCommand(m, "/login")

	if result.exitAction != ExitLogin {
		t.Errorf("exitAction = %d, want ExitLogin (%d)", result.exitAction, ExitLogin)
	}
	if !result.quitting {
		t.Error("quitting should be true after /login")
	}
}

func TestE2E_LogoutCommand_CallsLogoutFunc(t *testing.T) {
	logoutCalled := false
	m, _ := testModel(t, withLogoutFunc(func() error {
		logoutCalled = true
		return nil
	}))

	result, _ := submitCommand(m, "/logout")

	if !logoutCalled {
		t.Error("logoutFunc should have been called")
	}
	if !result.quitting {
		t.Error("quitting should be true after /logout")
	}
}

func TestE2E_LogoutCommand_NoLogoutFunc(t *testing.T) {
	m, _ := testModel(t) // logoutFunc is nil

	result, _ := submitCommand(m, "/logout")

	// Should still quit even if logoutFunc is nil.
	if !result.quitting {
		t.Error("quitting should be true after /logout even without logoutFunc")
	}
}

func TestE2E_LogoutCommand_ErrorDoesNotQuit(t *testing.T) {
	m, _ := testModel(t, withLogoutFunc(func() error {
		return errLogoutFailed
	}))

	result, _ := submitCommand(m, "/logout")

	// On error, should NOT quit.
	if result.quitting {
		t.Error("quitting should be false when logout fails")
	}
}

// errLogoutFailed is a sentinel error for testing.
var errLogoutFailed = &logoutError{}

type logoutError struct{}

func (e *logoutError) Error() string { return "logout failed" }
