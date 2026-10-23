package db

import (
	"os"
	"testing"
)

func TestLoadMigrationsSortedAndHasDown(t *testing.T) {
	migs, err := LoadMigrations(nil)
	if err != nil {
		t.Fatalf("LoadMigrations gagal: %v", err)
	}
	if len(migs) != 2 {
		t.Fatalf("jumlah migrasi = %d, mau 2", len(migs))
	}
	if migs[0].Version != 1 {
		t.Errorf("versi = %d, mau 1", migs[0].Version)
	}
	if stringsContains(migs[0].Up, "create table games") == false {
		t.Errorf("Up tidak memuat 'create table games'")
	}
	if stringsContains(migs[0].Down, "drop table") == false {
		t.Errorf("Down tidak memuat 'drop table'")
	}
	// Versi terurut.
	for i := 1; i < len(migs); i++ {
		if migs[i].Version <= migs[i-1].Version {
			t.Errorf("migrasi tidak terurut: %d setelah %d", migs[i].Version, migs[i-1].Version)
		}
	}
}

func TestLoadMigrationsBadName(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(dir+"/migrations", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/migrations/notanumber.sql", []byte("-- +migrate Down\nselect 1;"), 0o644); err != nil {
		t.Fatal(err)
	}
	fsys := os.DirFS(dir)
	_, err := LoadMigrations(fsys)
	if err == nil {
		t.Fatal("mau error untuk nama migrasi tanpa angka versi")
	}
}

func TestLoadMigrationsMissingDown(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(dir+"/migrations", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/migrations/0001_x.sql", []byte("create table t (id int);"), 0o644); err != nil {
		t.Fatal(err)
	}
	fsys := os.DirFS(dir)
	_, err := LoadMigrations(fsys)
	if err == nil {
		t.Fatal("mau error bila blok Down hilang")
	}
}

func stringsContains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
