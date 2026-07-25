package store

import "testing"

func TestLegacyMachinesGetAnOwnerOnStartup(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureBootstrapAdmin("admin", "adopt-test-pass"); err != nil {
		t.Fatal(err)
	}
	admin, err := st.Authenticate("admin", "adopt-test-pass")
	if err != nil {
		t.Fatal(err)
	}
	m, err := st.CreateMachine(admin.ID, MachineSpec{
		Name: "legacy", Address: "10.0.0.5", Port: 22, SSHUser: "ops",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Simulate a store written before machines carried an owner.
	st.mu.Lock()
	stale := st.machines[m.ID]
	stale.OwnerUserID = ""
	st.machines[m.ID] = stale
	if err := st.saveLocked(); err != nil {
		st.mu.Unlock()
		t.Fatal(err)
	}
	st.mu.Unlock()

	// Restart: the bootstrap pass should adopt the row.
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.EnsureBootstrapAdmin("admin", "adopt-test-pass"); err != nil {
		t.Fatal(err)
	}
	got, err := reopened.GetMachine(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.OwnerUserID != admin.ID {
		t.Fatalf("owner = %q, want the bootstrap admin %q — an owner-less row stays readable by every account", got.OwnerUserID, admin.ID)
	}

	// And it survives another restart without the migration undoing itself.
	again, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := again.EnsureBootstrapAdmin("admin", "adopt-test-pass"); err != nil {
		t.Fatal(err)
	}
	final, err := again.GetMachine(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.OwnerUserID != admin.ID {
		t.Fatalf("owner = %q after second restart, want %q", final.OwnerUserID, admin.ID)
	}
}
