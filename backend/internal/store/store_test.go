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

func TestMachineCRUD(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m, err := st.CreateMachine("user-1", "box", "10.0.0.5", 22, "root", "secret")
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

func TestTerminalSessions_underMachine(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m, err := st.CreateMachine("u1", "box", "ssh-target", 22, "root", "targetpass")
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
