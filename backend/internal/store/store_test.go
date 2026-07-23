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
	if err := st.EnsureBootstrapAdmin("akadal", "123456"); err != nil {
		t.Fatal(err)
	}
	// idempotent
	if err := st.EnsureBootstrapAdmin("akadal", "other"); err != nil {
		t.Fatal(err)
	}
	u, err := st.Authenticate("akadal", "123456")
	if err != nil {
		t.Fatal(err)
	}
	if u.Username != "akadal" || u.Role != "admin" {
		t.Fatalf("user=%+v", u)
	}
	if _, err := st.Authenticate("akadal", "nope"); err != store.ErrInvalidCreds {
		t.Fatalf("err=%v", err)
	}
}

func TestMachineCRUD(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m, err := st.CreateMachine("box", "10.0.0.5", 22, "root", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if m.ID == "" || m.Address != "10.0.0.5" {
		t.Fatalf("machine=%+v", m)
	}
	got, err := st.GetMachine(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.SSHPassword != "secret" {
		t.Fatal("password not persisted")
	}
	list := st.ListMachines()
	if len(list) != 1 {
		t.Fatalf("len=%d", len(list))
	}
	if err := st.DeleteMachine(m.ID); err != nil {
		t.Fatal(err)
	}
	if len(st.ListMachines()) != 0 {
		t.Fatal("expected empty")
	}
}

func TestTerminalSessions_underMachine(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m, err := st.CreateMachine("box", "ssh-target", 22, "root", "targetpass")
	if err != nil {
		t.Fatal(err)
	}
	t1, err := st.CreateTerminal(m.ID, "build")
	if err != nil {
		t.Fatal(err)
	}
	t2, err := st.CreateTerminal(m.ID, "debug")
	if err != nil {
		t.Fatal(err)
	}
	if t1.ID == t2.ID {
		t.Fatal("ids must differ")
	}
	if t1.MachineID != m.ID || t2.MachineID != m.ID {
		t.Fatal("machine_id mismatch")
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
