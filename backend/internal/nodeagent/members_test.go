package nodeagent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// nativeInviteModel mirrors member-helper.cjs over an in-memory invite store.
type nativeInviteModel struct {
	accounts []string
	codes    map[string]*nativeMemberCode
	order    []string
	minted   int
}

func (m *nativeInviteModel) usable() []string {
	result := []string{}
	for _, code := range m.order {
		if value := m.codes[code]; !value.Disabled && value.DID == "" {
			result = append(result, code)
		}
	}
	return result
}
func (m *nativeInviteModel) summary(tracked []string) nativeMembers {
	result := nativeMembers{Accounts: len(m.accounts), Codes: map[string]nativeMemberCode{}}
	if len(m.accounts) > 0 {
		result.Owner = m.accounts[0]
	}
	for _, code := range tracked {
		if value, ok := m.codes[code]; ok {
			result.Codes[code] = *value
		} else {
			result.Codes[code] = nativeMemberCode{}
		}
	}
	for _, code := range m.usable() {
		if slices.Contains(tracked, code) {
			result.Usable++
		}
	}
	return result
}
func (m *nativeInviteModel) sweep(tracked []string) {
	for _, code := range m.usable() {
		if !slices.Contains(tracked, code) {
			m.codes[code].Disabled = true
		}
	}
}
func (m *nativeInviteModel) run(args []string) nativeMembers {
	action, rest := args[0], args[1:]
	switch action {
	case "inventory":
		return m.summary(rest)
	case "disable":
		for _, code := range rest {
			if value, ok := m.codes[code]; ok {
				value.Disabled = true
			}
		}
		return m.summary(rest)
	case "sweep":
		if len(m.accounts) == 0 {
			result := m.summary(rest)
			result.OwnerRequired = true
			return result
		}
		m.sweep(rest)
		return m.summary(rest)
	case "issue":
		ceiling, _ := strconv.Atoi(rest[0])
		tracked := rest[1:]
		result := m.summary(tracked)
		if len(m.accounts) == 0 {
			result.OwnerRequired = true
			return result
		}
		m.sweep(tracked)
		if len(m.accounts)-1+len(m.usable()) >= ceiling {
			result = m.summary(tracked)
			result.CeilingReached = true
			return result
		}
		m.minted++
		code := fmt.Sprintf("member-code-%d", m.minted)
		m.codes[code] = &nativeMemberCode{Known: true}
		m.order = append(m.order, code)
		result = m.summary(tracked)
		result.Code = code
		return result
	}
	panic("unsupported helper action " + action)
}
func (m *nativeInviteModel) use(code, did, handle string) {
	m.codes[code].DID, m.codes[code].Handle = did, handle
	m.accounts = append(m.accounts, did)
}

func TestMemberInvitationsBoundCodesCeilingAndAdmission(t *testing.T) {
	id := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	ticket := func(n int) string { return fmt.Sprintf("%08d-bbbb-bbbb-bbbb-bbbbbbbbbbbb", n) }
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	runtime, err := NewDocker("/unused.sock", directory)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err = runtime.MemberInvitation(ctx, id, ticket(1)); err == nil {
		t.Fatal("member invitation accepted without explicit local fixtures")
	}
	runtime.EnableLocalIdentityFixtures()
	for _, args := range [][2]string{{"invalid", ticket(1)}, {id, "invalid"}} {
		if _, err = runtime.MemberInvitation(ctx, args[0], args[1]); err == nil {
			t.Fatal("unsafe member identity accepted")
		}
	}
	if err = runtime.SetMemberCeiling(0); err == nil || runtime.SetMemberCeiling(26) == nil {
		t.Fatal("member ceiling outside 1..25 accepted")
	}
	if err = runtime.SetMemberCeiling(2); err != nil {
		t.Fatal(err)
	}
	secret, err := runtime.credentials(id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = runtime.localCertificate(id); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"-edge", "-enrollment"} {
		if err = os.Mkdir(filepath.Join(directory, id+suffix), 0700); err != nil {
			t.Fatal(err)
		}
	}
	configPath := filepath.Join(directory, id+"-edge", "Caddyfile")
	tlsPath := filepath.Join(directory, id+"-tls.pem")
	if err = os.WriteFile(configPath, []byte(runtime.ingressConfiguration(id)), 0600); err != nil {
		t.Fatal(err)
	}
	network := "pds-ass-tenant-" + id + "-network"
	enrollmentDirectory := filepath.Join(directory, id+"-enrollment")
	running := true
	pds := map[string]any{
		"Config":          map[string]any{"Image": PDSImage, "Labels": map[string]string{label: id}, "Env": runtime.runtimeEnvironment(id, secret)},
		"HostConfig":      map[string]any{"Memory": 768 << 20, "NanoCpus": 1e9, "PidsLimit": 128, "NetworkMode": network, "CapDrop": []string{"ALL"}, "SecurityOpt": []string{"no-new-privileges:true"}},
		"Mounts":          []map[string]any{{"Type": "volume", "Name": "pds-ass-tenant-" + id + "-data", "Destination": "/pds", "RW": true}, {"Type": "volume", "Name": "pds-ass-tenant-" + id + "-blobs", "Destination": "/blobs", "RW": true}},
		"NetworkSettings": map[string]any{"Networks": map[string]any{network: map[string]any{}}},
	}
	edge := map[string]any{
		"Config":     map[string]any{"Image": CaddyImage, "User": privateFileUser(), "Env": ingressEnvironment(), "Labels": map[string]string{label: id, roleLabel: "ingress"}},
		"HostConfig": map[string]any{"Memory": 128 << 20, "NanoCpus": 250000000, "PidsLimit": 64, "NetworkMode": network, "CapDrop": []string{"ALL"}, "CapAdd": []string{"NET_BIND_SERVICE"}, "SecurityOpt": []string{"no-new-privileges:true"}, "Binds": []string{configPath + ":/etc/caddy/Caddyfile:ro", tlsPath + ":/etc/pds-tls.pem:ro"}},
		"Mounts":     []map[string]any{{"Type": "bind", "Source": configPath, "Destination": "/etc/caddy/Caddyfile"}, {"Type": "bind", "Source": tlsPath, "Destination": "/etc/pds-tls.pem"}},
		"State":      map[string]any{"Running": true}, "NetworkSettings": map[string]any{"Networks": map[string]any{network: map[string]any{}, "pds-ass-agent-ingress": map[string]any{}}},
	}
	guard := map[string]any{
		"Config":     map[string]any{"Image": PDSImage, "User": privateFileUser(), "Labels": map[string]string{label: id, roleLabel: "enrollment-guard"}, "Cmd": []string{"node", "--input-type=module", "-e", enrollmentGuardScript}, "Env": []string{"PDS_GUARD_INSTANCE=" + id}},
		"HostConfig": map[string]any{"Memory": 64 << 20, "NanoCpus": 125000000, "PidsLimit": 32, "ReadonlyRootfs": true, "NetworkMode": network, "CapDrop": []string{"ALL"}, "SecurityOpt": []string{"no-new-privileges:true"}, "Binds": []string{enrollmentDirectory + ":/enrollment:ro"}},
		"Mounts":     []map[string]any{{"Type": "bind", "Source": enrollmentDirectory, "Destination": "/enrollment"}},
		"State":      map[string]any{"Running": true}, "NetworkSettings": map[string]any{"Networks": map[string]any{network: map[string]any{}}},
	}
	native := &nativeInviteModel{codes: map[string]*nativeMemberCode{}}
	helpers := map[string][]string{}
	counter := 0
	loseIssue := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/v1.44")
		switch {
		case r.Method == "GET" && strings.HasPrefix(path, "/containers/"):
			var value map[string]any
			switch {
			case strings.Contains(path, "/pds-ass-tenant-"):
				value = pds
				value["State"] = map[string]any{"Running": running, "Health": map[string]string{"Status": "healthy"}}
			case strings.Contains(path, "/pds-ass-edge-"):
				value = edge
			case strings.Contains(path, "/pds-ass-enrollment-"):
				value = guard
			default:
				w.WriteHeader(404)
				return
			}
			_ = json.NewEncoder(w).Encode(value)
		case strings.HasSuffix(path, "/exec"):
			var body struct{ Cmd []string }
			if json.NewDecoder(r.Body).Decode(&body) != nil {
				t.Error("invalid fixed helper")
			}
			if strings.Contains(path, "/pds-ass-tenant-") && body.Cmd[3] != memberHelperScript {
				t.Error("unexpected native helper")
			}
			counter++
			helpers[fmt.Sprint(counter)] = body.Cmd
			_ = json.NewEncoder(w).Encode(map[string]string{"Id": fmt.Sprint(counter)})
		case strings.HasPrefix(path, "/exec/") && strings.HasSuffix(path, "/start"):
			args := helpers[strings.TrimSuffix(strings.TrimPrefix(path, "/exec/"), "/start")]
			var value any
			if strings.Contains(args[3], "/enrollment/members.json") {
				data, err := os.ReadFile(filepath.Join(enrollmentDirectory, "members.json"))
				if err != nil {
					t.Error(err)
				}
				hash := sha256.Sum256(data)
				value = map[string]string{"hash": hex.EncodeToString(hash[:])}
			} else {
				result := native.run(args[4:])
				if args[4] == "issue" && loseIssue {
					loseIssue = false
					w.WriteHeader(503)
					return
				}
				value = result
			}
			data, _ := json.Marshal(value)
			_, _ = w.Write(helperFrame(1, data))
		case strings.HasPrefix(path, "/exec/") && strings.HasSuffix(path, "/json"):
			_ = json.NewEncoder(w).Encode(map[string]any{"ExitCode": 0, "Running": false})
		default:
			t.Errorf("unexpected host operation %s %s", r.Method, path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	runtime.client = &http.Client{Transport: &rewriteTransport{base: server.URL, transport: http.DefaultTransport}}
	runtime.healthProbe = func(context.Context, string) bool { return true }
	view := func() memberAdmissionView {
		var value memberAdmissionView
		data, err := os.ReadFile(filepath.Join(enrollmentDirectory, "members.json"))
		if err != nil || json.Unmarshal(data, &value) != nil {
			t.Fatal("member admission view unavailable")
		}
		info, _ := os.Lstat(filepath.Join(enrollmentDirectory, "members.json"))
		if info.Mode().Perm()&0077 != 0 || strings.Contains(string(data), "member-code") {
			t.Fatal("member admission view is not private or exposes codes")
		}
		return value
	}

	// No owner account: nothing is minted and admission stays closed.
	waiting, err := runtime.MemberInvitation(ctx, id, ticket(1))
	if err != nil || waiting.State != "owner_required" || native.minted != 0 || len(view().Invitations) != 0 {
		t.Fatalf("owner requirement: %+v %v", waiting, err)
	}
	native.accounts = []string{"did:plc:aaaaaaaaaaaaaaaaaaaaaaaa"}

	// A lost issuance response leaves an untracked code; the retry disables it.
	loseIssue = true
	if _, err = runtime.MemberInvitation(ctx, id, ticket(2)); err == nil {
		t.Fatal("lost native response reported success")
	}
	if len(view().Invitations) != 0 {
		t.Fatal("failed issuance opened admission")
	}
	first, err := runtime.MemberInvitation(ctx, id, ticket(2))
	if err != nil || first.State != "invitation_ready" || first.Code != "member-code-2" || !native.codes["member-code-1"].Disabled {
		t.Fatalf("recovered issuance: %+v %v", first, err)
	}
	if !first.Valid(id, ticket(2), false) || first.ExpiresAt-time.Now().UnixMilli() > MemberInvitationLifetime.Milliseconds() {
		t.Fatal("issued invitation outside its native window")
	}
	replay, err := runtime.MemberInvitation(ctx, id, ticket(2))
	if err != nil || replay != first || native.minted != 2 {
		t.Fatal("replay minted a code or extended its window")
	}
	if admission := view(); len(admission.Invitations) != 1 || admission.Invitations[0].Ticket != ticket(2) || admission.Ceiling != 2 {
		t.Fatalf("admission view: %+v", admission)
	}

	// Owner plus one member plus one usable code reaches a ceiling of two.
	second, err := runtime.MemberInvitation(ctx, id, ticket(3))
	if err != nil || second.State != "invitation_ready" {
		t.Fatalf("second invitation: %+v %v", second, err)
	}
	full, err := runtime.MemberInvitation(ctx, id, ticket(4))
	if err != nil || full.State != "ceiling_reached" || native.minted != 3 {
		t.Fatalf("ceiling: %+v %v", full, err)
	}

	// A used code becomes an accepted member and leaves admission.
	native.use(first.Code, "did:plc:bbbbbbbbbbbbbbbbbbbbbbbb", "member.test")
	accepted, err := runtime.MemberInvitation(ctx, id, ticket(2))
	if err != nil || accepted.State != "accepted" || accepted.DID != "did:plc:bbbbbbbbbbbbbbbbbbbbbbbb" || accepted.Handle != "member.test" || accepted.Code != "" {
		t.Fatalf("acceptance: %+v %v", accepted, err)
	}
	if admission := view(); len(admission.Invitations) != 1 || admission.Invitations[0].Ticket != ticket(3) {
		t.Fatal("accepted invitation kept admission open")
	}

	// A code used just before withdrawal is reported as accepted, not withdrawn.
	native.use(second.Code, "did:plc:cccccccccccccccccccccccc", "raced.test")
	raced, err := runtime.WithdrawMemberInvitation(ctx, id, ticket(3))
	if err != nil || raced.State != "accepted" || !native.codes[second.Code].Disabled {
		t.Fatalf("withdrawal race: %+v %v", raced, err)
	}
	if _, err = runtime.WithdrawMemberInvitation(ctx, id, ticket(99)); err == nil {
		t.Fatal("unknown ticket withdrawal touched native state")
	}

	// Withdrawal disables an unused code and is idempotent.
	native.accounts = native.accounts[:1]
	third, err := runtime.MemberInvitation(ctx, id, ticket(5))
	if err != nil || third.State != "invitation_ready" {
		t.Fatalf("third invitation: %+v %v", third, err)
	}
	withdrawn, err := runtime.WithdrawMemberInvitation(ctx, id, ticket(5))
	if err != nil || withdrawn.State != "withdrawn" || !native.codes[third.Code].Disabled || len(view().Invitations) != 0 {
		t.Fatalf("withdrawal: %+v %v", withdrawn, err)
	}
	before := native.minted
	again, err := runtime.WithdrawMemberInvitation(ctx, id, ticket(5))
	if err != nil || again.State != "withdrawn" || native.minted != before {
		t.Fatal("repeated withdrawal changed native state")
	}
	if reopened, err := runtime.MemberInvitation(ctx, id, ticket(5)); err != nil || reopened.State != "withdrawn" || native.minted != before {
		t.Fatal("withdrawn ticket minted a code")
	}

	// Expired codes are disabled by the next broker call.
	fourth, err := runtime.MemberInvitation(ctx, id, ticket(6))
	if err != nil || fourth.State != "invitation_ready" {
		t.Fatal(err)
	}
	state, err := runtime.readMemberState(id)
	if err != nil {
		t.Fatal(err)
	}
	for i := range state.Entries {
		if state.Entries[i].Ticket == ticket(6) {
			state.Entries[i].ExpiresAt = time.Now().Add(-time.Second).UnixMilli()
			state.Entries[i].IssuedAt = state.Entries[i].ExpiresAt - 1000
		}
	}
	if err = runtime.writeMemberState(state); err != nil {
		t.Fatal(err)
	}
	expired, err := runtime.MemberInvitation(ctx, id, ticket(6))
	if err != nil || expired.State != "expired" || expired.Code != "" || !native.codes[fourth.Code].Disabled || len(view().Invitations) != 0 {
		t.Fatalf("expiry: %+v %v", expired, err)
	}
	info, err := os.Lstat(filepath.Join(directory, id+"-members.json"))
	if err != nil || info.Mode().Perm()&0077 != 0 {
		t.Fatal("private member state is not private")
	}

	// An unhealthy runtime refuses all member work.
	running = false
	if _, err = runtime.MemberInvitation(ctx, id, ticket(7)); err == nil {
		t.Fatal("unhealthy runtime issued a member code")
	}
}

func TestMemberResponseValidation(t *testing.T) {
	id, ticket := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	future := time.Now().Add(time.Hour).UnixMilli()
	valid := []MemberInvitation{
		{InstanceID: id, Ticket: ticket, State: "invitation_ready", Code: "code", ExpiresAt: future},
		{InstanceID: id, Ticket: ticket, State: "accepted", DID: "did:plc:aaaaaaaaaaaaaaaaaaaaaaaa", Handle: "member.test"},
		{InstanceID: id, Ticket: ticket, State: "expired", ExpiresAt: 1},
		{InstanceID: id, Ticket: ticket, State: "ceiling_reached"},
	}
	for _, value := range valid {
		if !value.Valid(id, ticket, false) {
			t.Fatalf("valid response rejected: %+v", value)
		}
	}
	invalid := []MemberInvitation{
		{InstanceID: ticket, Ticket: ticket, State: "withdrawn"},
		{InstanceID: id, Ticket: ticket, State: "invitation_ready", Code: "code", ExpiresAt: time.Now().Add(MemberInvitationLifetime + time.Minute).UnixMilli()},
		{InstanceID: id, Ticket: ticket, State: "accepted", DID: "did:web:example.com", Handle: "member.test"},
		{InstanceID: id, Ticket: ticket, State: "withdrawn", Code: "code"},
		{InstanceID: id, Ticket: ticket, State: "pending"},
	}
	for _, value := range invalid {
		if value.Valid(id, ticket, false) {
			t.Fatalf("invalid response accepted: %+v", value)
		}
	}
	if valid[0].Valid(id, ticket, true) {
		t.Fatal("withdrawal returned a usable code")
	}
}

type memberRuntimeFixture struct{ issued, withdrawn int }

func (*memberRuntimeFixture) Ensure(context.Context, string) (Instance, error) {
	return Instance{}, nil
}
func (m *memberRuntimeFixture) MemberInvitation(_ context.Context, id, ticket string) (MemberInvitation, error) {
	m.issued++
	return MemberInvitation{InstanceID: id, Ticket: ticket, State: "ceiling_reached"}, nil
}
func (m *memberRuntimeFixture) WithdrawMemberInvitation(_ context.Context, id, ticket string) (MemberInvitation, error) {
	m.withdrawn++
	return MemberInvitation{InstanceID: id, Ticket: ticket, State: "withdrawn"}, nil
}

func TestMemberRoutesAcceptOnlyFixedTicketRequests(t *testing.T) {
	runtime := &memberRuntimeFixture{}
	handler := Handler(runtime)
	id, ticket := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	call := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}
	base := "/v1/instances/" + id + "/members/invitation"
	for _, entry := range []struct {
		method, path, body string
		status             int
	}{
		{"POST", base, `{"ticket":"` + ticket + `"}`, 200},
		{"POST", base + "/withdraw", `{"ticket":"` + ticket + `"}`, 200},
		{"POST", base, `{"ticket":"invalid"}`, 400},
		{"POST", base, `{"ticket":"` + ticket + `","code":"chosen"}`, 400},
		{"POST", base + "?ceiling=25", `{"ticket":"` + ticket + `"}`, 404},
		{"GET", base, ``, 404},
		{"POST", "/v1/instances/not-a-uuid/members/invitation", `{"ticket":"` + ticket + `"}`, 404},
	} {
		if w := call(entry.method, entry.path, entry.body); w.Code != entry.status {
			t.Fatalf("%s %s: %d", entry.method, entry.path, w.Code)
		}
	}
	if runtime.issued != 1 || runtime.withdrawn != 1 {
		t.Fatalf("member routes dispatched incorrectly: %+v", runtime)
	}
}
