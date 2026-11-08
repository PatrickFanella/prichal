package nodeagent

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func writeManagedMail(t *testing.T, value any, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "managed-mail.json")
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestManagedMailAndEnrollmentAreHostOptIns(t *testing.T) {
	valid := ManagedMail{EmailFrom: "info@subcult.tv", SMTPURL: "smtps://user:secret@smtp-relay.brevo.com:465"}
	if err := pilotRuntime(t, t.TempDir()).EnableManagedMail(writeManagedMail(t, valid, 0600)); err == nil {
		t.Fatal("managed mail accepted without managed endpoints")
	}
	d := managedFixture(t)
	if err := d.EnableManagedEnrollment(); err == nil {
		t.Fatal("managed enrollment accepted without mail")
	}
	for name, path := range map[string]string{
		"relative path":  "managed-mail.json",
		"readable file":  writeManagedMail(t, valid, 0644),
		"unknown field":  writeManagedMail(t, map[string]string{"emailFrom": valid.EmailFrom, "smtpUrl": valid.SMTPURL, "hostname": "evil"}, 0600),
		"plain SMTP":     writeManagedMail(t, ManagedMail{EmailFrom: valid.EmailFrom, SMTPURL: "smtp://user:secret@smtp-relay.brevo.com:465"}, 0600),
		"STARTTLS port":  writeManagedMail(t, ManagedMail{EmailFrom: valid.EmailFrom, SMTPURL: "smtps://user:secret@smtp-relay.brevo.com:587"}, 0600),
		"no password":    writeManagedMail(t, ManagedMail{EmailFrom: valid.EmailFrom, SMTPURL: "smtps://user@smtp-relay.brevo.com:465"}, 0600),
		"display sender": writeManagedMail(t, ManagedMail{EmailFrom: "Prichal <info@subcult.tv>", SMTPURL: valid.SMTPURL}, 0600),
	} {
		t.Run(name, func(t *testing.T) {
			if err := d.EnableManagedMail(path); err == nil {
				t.Fatal("invalid managed mail policy accepted")
			}
		})
	}
	if err := d.EnableManagedMail(writeManagedMail(t, valid, 0600)); err != nil {
		t.Fatal(err)
	}
	if err := d.EnableManagedMail(writeManagedMail(t, valid, 0600)); err == nil {
		t.Fatal("managed mail policy replaced while configured")
	}
	if err := d.EnableManagedEnrollment(); err != nil || !d.ManagedEnrollment() {
		t.Fatal("managed enrollment refused after mail", err)
	}
}

func TestManagedEnrollmentShapesOnlyGeneratedInstances(t *testing.T) {
	d := managedFixture(t)
	retainManagedReceipt(t, d, managedFixtureID)
	secret, err := d.credentials(managedFixtureID)
	if err != nil {
		t.Fatal(err)
	}
	closed := d.ingressConfiguration(managedFixtureID)
	if !strings.Contains(closed, "Account creation is closed") || strings.Contains(closed, "forward_auth") {
		t.Fatal("managed edge opened without the enrollment opt-in")
	}
	for _, entry := range d.runtimeEnvironment(managedFixtureID, secret) {
		if strings.HasPrefix(entry, "PDS_EMAIL_") {
			t.Fatal("managed runtime gained mail without the opt-in")
		}
	}
	if d.guardFor(managedFixtureID) {
		t.Fatal("guard required without the enrollment opt-in")
	}
	if err := d.EnableManagedMail(writeManagedMail(t, ManagedMail{EmailFrom: "info@subcult.tv", SMTPURL: "smtps://user:secret@smtp-relay.brevo.com:465"}, 0600)); err != nil {
		t.Fatal(err)
	}
	if err := d.EnableManagedEnrollment(); err != nil {
		t.Fatal(err)
	}
	environment := d.runtimeEnvironment(managedFixtureID, secret)
	if !slices.Contains(environment, "PDS_EMAIL_FROM_ADDRESS=info@subcult.tv") || !slices.Contains(environment, "PDS_EMAIL_SMTP_URL=smtps://user:secret@smtp-relay.brevo.com:465") || !slices.Contains(environment, "PDS_DID_PLC_URL=https://plc.directory") {
		t.Fatal("managed runtime lacks real PLC or managed mail", environment)
	}
	guarded := d.ingressConfiguration(managedFixtureID)
	if strings.Contains(guarded, "Account creation is closed") || !strings.Contains(guarded, "forward_auth @registration pds-ass-enrollment-"+managedFixtureID+":2727") {
		t.Fatal("managed edge does not route signup through the first-account guard")
	}
	host := "pds-2395105c995847b5ae8b5b0424cbfc4f.subcult.tv"
	if !d.guardFor(managedFixtureID) || !slices.Contains(d.guardEnvironment(managedFixtureID), "PDS_GUARD_FIRST_ACCOUNT_ONLY=true") {
		t.Fatal("managed guard is not first-account only")
	}
	if got := d.enrollmentArguments(managedFixtureID, "invitation"); !slices.Equal(got, []string{"invitation", host}) {
		t.Fatalf("managed helper arguments %v", got)
	}
	if d.tenantNetwork(managedFixtureID) != "pds-ass-managed-tenant-"+managedFixtureID+"-network" {
		t.Fatal("managed guard would join the private tenant network")
	}
	private := "bbbbbbbb-bbbb-4bbb-bbbb-bbbbbbbbbbbb"
	if d.guardFor(private) || len(d.enrollmentArguments(private, "inventory")) != 1 {
		t.Fatal("managed enrollment widened to a private instance")
	}
}

type managedEnrollmentRuntime struct {
	enabled bool
	enrolls int
}

func (r *managedEnrollmentRuntime) ManagedProvisioningOnly() bool       { return true }
func (r *managedEnrollmentRuntime) ManagedIdentityRetained(string) bool { return true }
func (r *managedEnrollmentRuntime) ManagedEnrollment() bool             { return r.enabled }
func (r *managedEnrollmentRuntime) Ensure(context.Context, string) (Instance, error) {
	return Instance{}, nil
}
func (r *managedEnrollmentRuntime) Enroll(_ context.Context, id, ticket string) (Enrollment, error) {
	r.enrolls++
	return Enrollment{InstanceID: id, Ticket: ticket, State: "invitation_ready"}, nil
}

func TestManagedEnrollmentRouteFence(t *testing.T) {
	id := "2395105c-9958-47b5-ae8b-5b0424cbfc4f"
	body := `{"ticket":"aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa"}`
	for _, enabled := range []bool{false, true} {
		runtime := &managedEnrollmentRuntime{enabled: enabled}
		for _, test := range []struct {
			method, path string
			open         bool
		}{
			{"POST", "/v1/instances/" + id + "/enrollment", true},
			{"POST", "/v1/instances/" + id + "/enrollment?x=1", false},
			{"POST", "/v1/instances/" + id + "/service", false},
			{"POST", "/v1/instances/" + id + "/members/invitation", false},
			{"POST", "/v1/instances/" + id + "/export", false},
		} {
			w := httptest.NewRecorder()
			Handler(runtime).ServeHTTP(w, httptest.NewRequest(test.method, test.path, strings.NewReader(body)))
			if want := enabled && test.open; (w.Code == 200) != want {
				t.Fatalf("enabled=%v %s %s: %d", enabled, test.method, test.path, w.Code)
			}
		}
		if want := map[bool]int{false: 0, true: 1}[enabled]; runtime.enrolls != want {
			t.Fatalf("enabled=%v enrollments %d", enabled, runtime.enrolls)
		}
	}
}
