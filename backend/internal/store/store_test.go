package store_test

import (
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
