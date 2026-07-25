package store_test

import (
	"errors"
	"testing"

	"github.com/akadal/agent-hub/backend/internal/store"
)

func TestBootstrapAndAuthenticate(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureBootstrapAdmin("admin", "123456"); err != nil {
		t.Fatal(err)
	}
	u, err := st.Authenticate("admin", "123456")
	if err != nil {
		t.Fatal(err)
	}
	if u.Username != "admin" || u.Role != "admin" {
		t.Fatalf("user=%+v", u)
	}
	if _, err := st.Authenticate("admin", "nope"); err != store.ErrInvalidCreds {
		t.Fatalf("err=%v", err)
	}
	// bootstrap re-syncs password from env on each call (Coolify credential updates)
	if err := st.EnsureBootstrapAdmin("admin", "newpass"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Authenticate("admin", "123456"); err != store.ErrInvalidCreds {
		t.Fatal("old password should fail after sync")
	}
	if _, err := st.Authenticate("admin", "newpass"); err != nil {
		t.Fatalf("new password: %v", err)
	}
}

func TestUserCRUD(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureBootstrapAdmin("admin", "adminpass"); err != nil {
		t.Fatal(err)
	}

	// create regular user
	u, err := st.CreateUser("alice", "alicepass", store.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	if u.ID == "" || u.Username != "alice" || u.Role != store.RoleUser {
		t.Fatalf("user=%+v", u)
	}
	if u.PasswordHash == "" || u.PasswordHash == "alicepass" {
		t.Fatal("password must be hashed, not stored plaintext")
	}

	// authenticate new user
	got, err := st.Authenticate("alice", "alicepass")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != u.ID || got.Role != store.RoleUser {
		t.Fatalf("auth user=%+v", got)
	}

	// unique username
	if _, err := st.CreateUser("alice", "other", store.RoleUser); err != store.ErrUserExists {
		t.Fatalf("want ErrUserExists, got %v", err)
	}

	// list includes bootstrap + alice
	list := st.ListUsers()
	if len(list) != 2 {
		t.Fatalf("list len=%d", len(list))
	}

	// update password
	if _, err := st.UpdateUser(u.ID, "newpass", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Authenticate("alice", "alicepass"); err != store.ErrInvalidCreds {
		t.Fatal("old password should fail")
	}
	if _, err := st.Authenticate("alice", "newpass"); err != nil {
		t.Fatalf("new password: %v", err)
	}

	// promote to admin
	upd, err := st.UpdateUser(u.ID, "", store.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if upd.Role != store.RoleAdmin {
		t.Fatalf("role=%s", upd.Role)
	}

	// public view never exposes hash
	pub := upd.Public()
	if pub.Username != "alice" || pub.Role != store.RoleAdmin {
		t.Fatalf("public=%+v", pub)
	}

	// delete non-last admin ok when another admin exists
	if err := st.DeleteUser(u.ID); err != nil {
		t.Fatal(err)
	}
	if len(st.ListUsers()) != 1 {
		t.Fatal("expected only bootstrap admin left")
	}
}

func TestUser_lastAdminProtection(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureBootstrapAdmin("admin", "adminpass"); err != nil {
		t.Fatal(err)
	}
	admins := st.ListUsers()
	if len(admins) != 1 {
		t.Fatalf("want 1 user, got %d", len(admins))
	}
	id := admins[0].ID

	if err := st.DeleteUser(id); err != store.ErrLastAdmin {
		t.Fatalf("delete last admin: got %v", err)
	}
	if _, err := st.UpdateUser(id, "", store.RoleUser); err != store.ErrLastAdmin {
		t.Fatalf("demote last admin: got %v", err)
	}
	// still can change password
	if _, err := st.UpdateUser(id, "rotated", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Authenticate("admin", "rotated"); err != nil {
		t.Fatal(err)
	}
}

func TestUser_invalidRole(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateUser("bob", "pass", "superuser"); err != store.ErrInvalidRole {
		t.Fatalf("got %v", err)
	}
}

func TestMachineCRUD(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m, err := st.CreateMachine("user-1", store.MachineSpec{
		Name: "box", Address: "10.0.0.5", Port: 22, SSHUser: "root", SSHPassword: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.ID == "" || m.Address != "10.0.0.5" || m.OwnerUserID != "user-1" {
		t.Fatalf("machine=%+v", m)
	}
	got, err := st.GetMachine(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.SSHPassword != "secret" {
		t.Fatal("password not persisted")
	}
	list := st.ListMachines("user-1")
	if len(list) != 1 {
		t.Fatalf("len=%d", len(list))
	}
	if err := st.DeleteMachine(m.ID); err != nil {
		t.Fatal(err)
	}
	if len(st.ListMachines("user-1")) != 0 {
		t.Fatal("expected empty")
	}
}

func TestMachineGrants_andAccess(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureBootstrapAdmin("admin", "adminpass"); err != nil {
		t.Fatal(err)
	}
	alice, err := st.CreateUser("alice", "pass", store.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := st.CreateUser("bob", "pass", store.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	m, err := st.CreateMachine(alice.ID, store.MachineSpec{
		Name: "box", Address: "10.0.0.1", Port: 22, SSHUser: "root", SSHPassword: "x",
	})
	if err != nil {
		t.Fatal(err)
	}

	// owner can access; bob cannot until granted
	if !st.UserCanAccessMachine(alice.ID, store.RoleUser, m) {
		t.Fatal("owner should access")
	}
	if st.UserCanAccessMachine(bob.ID, store.RoleUser, m) {
		t.Fatal("bob should not access yet")
	}
	if _, err := st.GrantMachineAccess(bob.ID, m.ID); err != nil {
		t.Fatal(err)
	}
	if !st.UserCanAccessMachine(bob.ID, store.RoleUser, m) {
		t.Fatal("bob should access after grant")
	}
	// list for bob includes granted machine
	list := st.ListMachinesForUser(bob.ID, store.RoleUser)
	if len(list) != 1 || list[0].ID != m.ID {
		t.Fatalf("bob list=%+v", list)
	}
	// bob cannot manage (delete)
	if st.UserCanManageMachine(bob.ID, store.RoleUser, m) {
		t.Fatal("grantee must not manage")
	}
	if err := st.RevokeMachineAccess(bob.ID, m.ID); err != nil {
		t.Fatal(err)
	}
	if st.UserCanAccessMachine(bob.ID, store.RoleUser, m) {
		t.Fatal("revoked")
	}
	// admin sees all
	admins := st.ListUsers()
	adminID := admins[0].ID
	if len(st.ListMachinesForUser(adminID, store.RoleAdmin)) != 1 {
		t.Fatal("admin should list machine")
	}
}

func TestAudit_andSettings(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AppendAudit(store.AuditEvent{Username: "a", Action: "login.ok"}); err != nil {
		t.Fatal(err)
	}
	if err := st.AppendAudit(store.AuditEvent{Username: "b", Action: "machine.create"}); err != nil {
		t.Fatal(err)
	}
	ev := st.ListAudit(10)
	if len(ev) != 2 {
		t.Fatalf("len=%d", len(ev))
	}
	// newest first
	if ev[0].Action != "machine.create" {
		t.Fatalf("want newest first, got %s", ev[0].Action)
	}
	stt := st.GetSettings()
	if stt.NetworkMode != store.NetworkPrivateMesh {
		t.Fatalf("default=%s", stt.NetworkMode)
	}
	stt, err = st.UpdateSettings(store.NetworkOpen)
	if err != nil || stt.NetworkMode != store.NetworkOpen {
		t.Fatalf("update: %+v %v", stt, err)
	}
	// reload from disk
	st2, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if st2.GetSettings().NetworkMode != store.NetworkOpen {
		t.Fatal("settings not persisted")
	}
	if len(st2.ListAudit(5)) != 2 {
		t.Fatal("audit not persisted")
	}
}

func TestTerminalSessions_underMachine(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m, err := st.CreateMachine("u1", store.MachineSpec{
		Name: "box", Address: "ssh-target", Port: 22, SSHUser: "root", SSHPassword: "targetpass",
	})
	if err != nil {
		t.Fatal(err)
	}
	t1, err := st.CreateTerminal(m.ID, "u1", "build")
	if err != nil {
		t.Fatal(err)
	}
	t2, err := st.CreateTerminal(m.ID, "u1", "debug")
	if err != nil {
		t.Fatal(err)
	}
	if t1.ID == t2.ID {
		t.Fatal("ids must differ")
	}
	if t1.MachineID != m.ID || t2.MachineID != m.ID {
		t.Fatal("machine_id mismatch")
	}
	if t1.RemoteSession == "" || t1.RemoteSession == t2.RemoteSession {
		t.Fatalf("remote sessions must be unique non-empty: %q %q", t1.RemoteSession, t2.RemoteSession)
	}
	list, err := st.ListTerminalsByMachine(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("len=%d", len(list))
	}
	if err := st.CloseTerminal(t1.ID); err != nil {
		t.Fatal(err)
	}
	list, err = st.ListTerminalsByMachine(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != t2.ID {
		t.Fatalf("after close: %+v", list)
	}
	// deleting machine removes remaining sessions
	if err := st.DeleteMachine(m.ID); err != nil {
		t.Fatal(err)
	}
	if len(st.ListTerminals()) != 0 {
		t.Fatal("expected no terminals after machine delete")
	}
}

// A pin is recorded once and never silently replaced: overwriting a stored host
// key is precisely what an attacker swapping in their own server would want.
func TestPinMachineHostKeyRecordsOnceAndRefusesToOverwrite(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m, err := st.CreateMachine("owner", store.MachineSpec{Name: "box", Address: "10.0.0.5"})
	if err != nil {
		t.Fatal(err)
	}
	if m.HostKeyFingerprint != "" {
		t.Fatal("a new machine must start unpinned so the first connect can learn the key")
	}

	pinned, err := st.PinMachineHostKey(m.ID, "SHA256:original")
	if err != nil || !pinned {
		t.Fatalf("first pin: pinned=%v err=%v", pinned, err)
	}
	got, _ := st.GetMachine(m.ID)
	if got.HostKeyFingerprint != "SHA256:original" {
		t.Fatalf("fingerprint = %q, want SHA256:original", got.HostKeyFingerprint)
	}

	pinned, err = st.PinMachineHostKey(m.ID, "SHA256:impostor")
	if err != nil {
		t.Fatal(err)
	}
	if pinned {
		t.Fatal("a second pin must not report a write")
	}
	got, _ = st.GetMachine(m.ID)
	if got.HostKeyFingerprint != "SHA256:original" {
		t.Fatalf("pin was overwritten to %q — a rebuilt host must go through delete-and-re-register",
			got.HostKeyFingerprint)
	}

	// An empty observation is a no-op, not a way to clear the pin.
	if _, err := st.PinMachineHostKey(m.ID, ""); err != nil {
		t.Fatal(err)
	}
	got, _ = st.GetMachine(m.ID)
	if got.HostKeyFingerprint != "SHA256:original" {
		t.Fatal("an empty fingerprint must not clear the pin")
	}
}

// The pin must survive a restart, or every reboot silently re-trusts whatever
// answers next.
func TestPinnedHostKeySurvivesReload(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := st.CreateMachine("owner", store.MachineSpec{Name: "box", Address: "10.0.0.5"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.PinMachineHostKey(m.ID, "SHA256:persisted"); err != nil {
		t.Fatal(err)
	}

	reopened, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.GetMachine(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.HostKeyFingerprint != "SHA256:persisted" {
		t.Fatalf("after reload fingerprint = %q, want SHA256:persisted", got.HostKeyFingerprint)
	}
}

// The bootstrap admin is the account most likely to still be on the published
// demo password, so "change it in the UI" has to hold across a restart.
func TestSelfChangedAdminPasswordSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureBootstrapAdmin("admin", "from-env"); err != nil {
		t.Fatal(err)
	}
	u, err := st.Authenticate("admin", "from-env")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ChangePassword(u.ID, "from-env", "chosen-in-ui"); err != nil {
		t.Fatal(err)
	}

	// Same env value as before: a restart must not undo the operator's change.
	restarted, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.EnsureBootstrapAdmin("admin", "from-env"); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Authenticate("admin", "chosen-in-ui"); err != nil {
		t.Fatalf("password chosen in the UI did not survive restart: %v", err)
	}
	if _, err := restarted.Authenticate("admin", "from-env"); err == nil {
		t.Fatal("the old env password still works after the admin changed it")
	}
}

// The env password is also the documented recovery path: change it and restart.
func TestChangedEnvPasswordIsReappliedOnRestart(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureBootstrapAdmin("admin", "from-env"); err != nil {
		t.Fatal(err)
	}
	u, _ := st.Authenticate("admin", "from-env")
	if err := st.ChangePassword(u.ID, "from-env", "forgotten"); err != nil {
		t.Fatal(err)
	}

	restarted, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.EnsureBootstrapAdmin("admin", "operator-reset"); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Authenticate("admin", "operator-reset"); err != nil {
		t.Fatalf("recovery via BOOTSTRAP_ADMIN_PASSWORD broke: %v", err)
	}
}

func TestChangePasswordRequiresTheCurrentOne(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	u, err := st.CreateUser("dev", "old-secret", "user")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ChangePassword(u.ID, "guessed", "new-secret"); !errors.Is(err, store.ErrInvalidCreds) {
		t.Fatalf("wrong current password: err = %v, want ErrInvalidCreds", err)
	}
	if _, err := st.Authenticate("dev", "old-secret"); err != nil {
		t.Fatal("a failed change must not touch the stored password")
	}
	if err := st.ChangePassword(u.ID, "old-secret", ""); !errors.Is(err, store.ErrInvalidPassword) {
		t.Fatalf("empty new password: err = %v, want ErrInvalidPassword", err)
	}
	if err := st.ChangePassword(u.ID, "old-secret", "new-secret"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Authenticate("dev", "new-secret"); err != nil {
		t.Fatalf("new password does not authenticate: %v", err)
	}
}
