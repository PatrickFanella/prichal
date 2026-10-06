package main

import "testing"

func TestEnrollmentListenerKeyTransition(t *testing.T) {
	t.Setenv("PDS_ENROLLMENT_KEY_FILE", "/legacy/listener.key")
	t.Setenv("PDS_ENROLLMENT_LISTENER_KEY_FILE", "")
	if got := enrollmentListenerKeyFile(); got != "/legacy/listener.key" {
		t.Fatalf("legacy listener key = %q", got)
	}
	t.Setenv("PDS_ENROLLMENT_LISTENER_KEY_FILE", "/tls/listener.key")
	t.Setenv("PDS_ENROLLMENT_KEY_FILE", "/management/enrollment.key")
	if got := enrollmentListenerKeyFile(); got != "/tls/listener.key" {
		t.Fatalf("listener received invitation key: %q", got)
	}
}
