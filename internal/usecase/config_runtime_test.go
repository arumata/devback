package usecase

import "testing"

func TestRuntimeConfigFromFile_Defaults(t *testing.T) {
	cfg := DefaultConfigFile()
	got, err := RuntimeConfigFromFile(cfg, "/home/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.BackupDir != "" {
		t.Fatalf("unexpected backup dir: %s", got.BackupDir)
	}
	if got.KeepCount != cfg.Backup.KeepCount {
		t.Fatalf("unexpected keep count: %d", got.KeepCount)
	}
	if got.KeepDays != cfg.Backup.KeepDays {
		t.Fatalf("unexpected keep days: %d", got.KeepDays)
	}
	if got.MaxTotalGBPerRepo != cfg.Backup.MaxTotalGB {
		t.Fatalf("unexpected max total gb: %d", got.MaxTotalGBPerRepo)
	}
	if got.RepoKeyStyle != cfg.RepoKey.Style {
		t.Fatalf("unexpected repo key style: %s", got.RepoKeyStyle)
	}
	if got.AutoRemoteMerge != cfg.RepoKey.AutoRemoteMerge {
		t.Fatalf("unexpected auto remote merge: %t", got.AutoRemoteMerge)
	}
	if got.RemoteHashLen != cfg.RepoKey.RemoteHashLen {
		t.Fatalf("unexpected remote hash len: %d", got.RemoteHashLen)
	}
	if got.SizeMarginMB != cfg.Backup.SizeMarginMB {
		t.Fatalf("unexpected size margin: %d", got.SizeMarginMB)
	}
	if got.NoSize != cfg.Backup.NoSize {
		t.Fatalf("unexpected no size flag: %t", got.NoSize)
	}
}

func TestRuntimeConfigFromFile_EmptyHome(t *testing.T) {
	_, err := RuntimeConfigFromFile(DefaultConfigFile(), "")
	if err == nil {
		t.Fatal("expected error")
	}
}
