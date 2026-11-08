package managedrouting

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managededge"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managednamespace"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
)

const invitationOne = "49aac16a-2663-4eb4-a95d-4c5c3029506e"
const accountDID = "did:plc:eqtyxf4i56sfx4plgd2necjd"

type handleInspector struct {
	result nodeagent.Enrollment
	calls  int
}

func (h *handleInspector) Enroll(_ context.Context, _ nodeagent.Peer, id, ticket string) (nodeagent.Enrollment, error) {
	h.calls++
	result := h.result
	result.InstanceID, result.Ticket = id, ticket
	return result, nil
}

type documentResolver struct{ document DIDDocument }

func (d documentResolver) Resolve(context.Context, string) (DIDDocument, error) {
	return d.document, nil
}

// recordEnrollment stores the registry enrollment that a handle binding must name.
func recordEnrollment(t *testing.T, f storeFixture, deployment, invitation, state string) {
	t.Helper()
	if _, err := f.Pool.Exec(context.Background(), `INSERT INTO enrollment_operations(id,deployment_id,owner_id,idempotency_key,state) SELECT $1,d.id,o.owner_id,gen_random_uuid(),$3 FROM deployments d JOIN organizations o ON o.id=d.organization_id WHERE d.id=$2`, invitation, deployment, state); err != nil {
		t.Fatal(err)
	}
}

func readyEndpoint(t *testing.T) (storeFixture, string) {
	t.Helper()
	f := routingFixture(t)
	ctx := context.Background()
	if _, err := f.Reserve(ctx, "operator", f.id); err != nil {
		t.Fatal(err)
	}
	f.place(t)
	host, _ := managednamespace.Endpoint(f.id)
	return f, host
}

func TestHandleBindingRequiresNativeAndPublicAgreement(t *testing.T) {
	f, host := readyEndpoint(t)
	ctx := context.Background()
	handle, _ := managednamespace.Handle(invitationOne)
	good := nodeagent.Enrollment{State: "verified", DID: accountDID, Handle: handle, EmailConfirmed: true}
	document := DIDDocument{AlsoKnownAs: []string{"at://" + handle}, PDSEndpoint: "https://" + host}
	recordEnrollment(t, f, f.id, invitationOne, "account_created")
	for name, test := range map[string]struct {
		native   nodeagent.Enrollment
		document DIDDocument
	}{
		"no account":           {nodeagent.Enrollment{State: "invitation_ready"}, document},
		"other handle":         {nodeagent.Enrollment{State: "verified", DID: accountDID, Handle: "alice.subcult.tv"}, document},
		"malformed DID":        {nodeagent.Enrollment{State: "verified", DID: "did:web:evil.example", Handle: handle}, document},
		"PLC claims elsewhere": {good, DIDDocument{AlsoKnownAs: []string{"at://alice.subcult.tv", "at://" + handle}, PDSEndpoint: "https://" + host}},
		"PLC other PDS":        {good, DIDDocument{AlsoKnownAs: []string{"at://" + handle}, PDSEndpoint: "https://pds.example.net"}},
		"PLC no handle":        {good, DIDDocument{PDSEndpoint: "https://" + host}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := f.BindHandle(ctx, "operator", f.id, f.node, invitationOne, &handleInspector{result: test.native}, documentResolver{test.document}); !errors.Is(err, ErrUnqualified) {
				t.Fatal("unverified binding accepted", err)
			}
		})
	}
	other := "bbbbbbbb-bbbb-4bbb-abbb-bbbbbbbbbbbb"
	recordEnrollment(t, f, f.id, other, "verified")
	if _, err := f.BindHandle(ctx, "operator", f.id, other, invitationOne, &handleInspector{result: good}, documentResolver{document}); !errors.Is(err, ErrConflict) {
		t.Fatal("binding accepted for another node", err)
	}
	inspector := &handleInspector{result: good}
	binding, err := f.BindHandle(ctx, "operator", f.id, f.node, invitationOne, inspector, documentResolver{document})
	if err != nil || binding.Hostname != "at-49aac16a26634eb.subcult.tv" || binding.DID != accountDID || binding.DeploymentID != f.id {
		t.Fatal("verified binding refused", binding, err)
	}
	again, err := f.BindHandle(ctx, "operator", f.id, f.node, invitationOne, inspector, documentResolver{})
	if err != nil || again != binding || inspector.calls != 1 {
		t.Fatal("binding replay reverified or changed", err)
	}
	if _, err := f.BindHandle(ctx, "operator", f.id, f.node, other, &handleInspector{result: nodeagent.Enrollment{State: "verified", DID: accountDID, Handle: mustHandle(other)}}, documentResolver{DIDDocument{AlsoKnownAs: []string{"at://" + mustHandle(other)}, PDSEndpoint: "https://" + host}}); !errors.Is(err, ErrConflict) {
		t.Fatal("one DID bound to two handles", err)
	}
	if _, err := f.Observe(ctx, "operator", f.id, f.node, requestOne, f.inspector); err != nil {
		t.Fatal(err)
	}
	bundle, err := f.Export(ctx)
	if err != nil || len(bundle.Inventory.Handles) != 1 {
		t.Fatal("verified handle not exported", err)
	}
	if h := bundle.Inventory.Handles[0]; h != (managededge.Handle{InvitationID: invitationOne, DeploymentID: f.id, Hostname: binding.Hostname, DID: accountDID}) {
		t.Fatal("exported handle differs", h)
	}
	directory := filepath.Join(t.TempDir(), "bundle")
	if err := WriteBundle(directory, bundle); err != nil {
		t.Fatal(err)
	}
	if _, err := managededge.Render(bundle.Inventory, filepath.Join(directory, "certificates"), time.Now()); err != nil {
		t.Fatal("exported handle does not satisfy the candidate contract", err)
	}
}

func TestHandleBindingRefusesUnreadyOrRetiredEndpoints(t *testing.T) {
	f := routingFixture(t)
	ctx := context.Background()
	if _, err := f.Reserve(ctx, "operator", f.id); err != nil {
		t.Fatal(err)
	}
	handle := mustHandle(invitationOne)
	native := &handleInspector{result: nodeagent.Enrollment{State: "verified", DID: accountDID, Handle: handle}}
	if _, err := f.BindHandle(ctx, "operator", f.id, f.node, invitationOne, native, documentResolver{}); !errors.Is(err, ErrUnqualified) || native.calls != 0 {
		t.Fatal("unplaced endpoint reached the native inspector", err)
	}
	for _, bad := range []string{"", "not-a-uuid", "49AAC16A-2663-4EB4-A95D-4C5C3029506E"} {
		if _, err := f.BindHandle(ctx, "operator", f.id, f.node, bad, native, documentResolver{}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid invitation %q accepted", bad)
		}
	}
}

func TestHandleBindingNamesOnlyARecordedEnrollment(t *testing.T) {
	f, host := readyEndpoint(t)
	ctx := context.Background()
	pending := "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	foreign := "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	recordEnrollment(t, f, f.id, pending, "pending")
	var second string
	if err := f.Pool.QueryRow(ctx, `INSERT INTO deployments(id,organization_id,name,region,idempotency_key,kind,state,managed_hostname,endpoint_url) SELECT gen_random_uuid(),organization_id,'second','local','second','agent','queued',gen_random_uuid()::text||'.localhost','https://second.localhost:44444' FROM deployments WHERE id=$1 RETURNING id`, f.id).Scan(&second); err != nil {
		t.Fatal(err)
	}
	recordEnrollment(t, f, second, foreign, "verified")
	for name, invitation := range map[string]string{"unrecorded": invitationOne, "no account yet": pending, "other deployment": foreign} {
		t.Run(name, func(t *testing.T) {
			handle := mustHandle(invitation)
			native := &handleInspector{result: nodeagent.Enrollment{State: "verified", DID: accountDID, Handle: handle}}
			document := DIDDocument{AlsoKnownAs: []string{"at://" + handle}, PDSEndpoint: "https://" + host}
			if _, err := f.BindHandle(ctx, "operator", f.id, f.node, invitation, native, documentResolver{document}); !errors.Is(err, ErrUnqualified) || native.calls != 0 {
				t.Fatal("unrecorded invitation reached the native enrollment replay", err, native.calls)
			}
		})
	}
}

func mustHandle(invitation string) string {
	handle, err := managednamespace.Handle(invitation)
	if err != nil {
		panic(err)
	}
	return handle
}

func TestPLCResolverReadsOnlyTheRequestedDocument(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" || r.URL.Path != "/"+accountDID {
			w.WriteHeader(404)
			return
		}
		w.Write([]byte(`{"id":"` + accountDID + `","alsoKnownAs":["at://at-49aac16a26634eb.subcult.tv"],"service":[{"id":"#atproto_pds","type":"AtprotoPersonalDataServer","serviceEndpoint":"https://pds-e4067ea1f0004321852f0c67a179c0ed.subcult.tv"}]}`))
	}))
	defer server.Close()
	resolver := PLCResolver{Client: server.Client(), BaseURL: server.URL}
	document, err := resolver.Resolve(context.Background(), accountDID)
	if err != nil || document.AlsoKnownAs[0] != "at://at-49aac16a26634eb.subcult.tv" || document.PDSEndpoint != "https://pds-e4067ea1f0004321852f0c67a179c0ed.subcult.tv" {
		t.Fatal(document, err)
	}
	if _, err := resolver.Resolve(context.Background(), "did:plc:aaaaaaaaaaaaaaaaaaaaaaaa"); err == nil {
		t.Fatal("missing document resolved")
	}
	if _, err := resolver.Resolve(context.Background(), "did:web:evil.example"); !errors.Is(err, ErrInvalid) {
		t.Fatal("non-PLC DID resolved")
	}
}
