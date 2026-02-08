package usecase

import (
	"reflect"
	"testing"
)

func TestConfig_DefaultValues(t *testing.T) {
	cfg := &Config{}

	// Test zero values for various fields
	if cfg.BackupDir != "" {
		t.Errorf("Expected empty BackupDir, got %s", cfg.BackupDir)
	}
	if cfg.Verbose != false {
		t.Errorf("Expected Verbose to be false, got %v", cfg.Verbose)
	}
	if cfg.DryRun != false {
		t.Errorf("Expected DryRun to be false, got %v", cfg.DryRun)
	}
	if cfg.PrintRepoKey != false {
		t.Errorf("Expected PrintRepoKey to be false, got %v", cfg.PrintRepoKey)
	}
	if cfg.TestLocks != false {
		t.Errorf("Expected TestLocks to be false, got %v", cfg.TestLocks)
	}
	if cfg.KeepCount != 0 {
		t.Errorf("Expected KeepCount to be 0, got %d", cfg.KeepCount)
	}
	if cfg.KeepDays != 0 {
		t.Errorf("Expected KeepDays to be 0, got %d", cfg.KeepDays)
	}
	if cfg.MaxTotalGBPerRepo != 0 {
		t.Errorf("Expected MaxTotalGBPerRepo to be 0, got %d", cfg.MaxTotalGBPerRepo)
	}
	if cfg.SizeMarginMB != 0 {
		t.Errorf("Expected SizeMarginMB to be 0, got %d", cfg.SizeMarginMB)
	}
}

func TestConfig_BasicFields(t *testing.T) {
	cfg := &Config{
		BackupDir:    "/test/backup",
		Verbose:      true,
		DryRun:       true,
		PrintRepoKey: true,
		TestLocks:    true,
	}

	if cfg.BackupDir != "/test/backup" {
		t.Errorf("Expected BackupDir '/test/backup', got %s", cfg.BackupDir)
	}
	if cfg.Verbose != true {
		t.Errorf("Expected Verbose to be true, got %v", cfg.Verbose)
	}
	if cfg.DryRun != true {
		t.Errorf("Expected DryRun to be true, got %v", cfg.DryRun)
	}
	if cfg.PrintRepoKey != true {
		t.Errorf("Expected PrintRepoKey to be true, got %v", cfg.PrintRepoKey)
	}
	if cfg.TestLocks != true {
		t.Errorf("Expected TestLocks to be true, got %v", cfg.TestLocks)
	}
}

func TestConfig_RetentionFields(t *testing.T) {
	cfg := &Config{
		KeepCount:         30,
		KeepDays:          90,
		MaxTotalGBPerRepo: 10,
	}

	if cfg.KeepCount != 30 {
		t.Errorf("Expected KeepCount 30, got %d", cfg.KeepCount)
	}
	if cfg.KeepDays != 90 {
		t.Errorf("Expected KeepDays 90, got %d", cfg.KeepDays)
	}
	if cfg.MaxTotalGBPerRepo != 10 {
		t.Errorf("Expected MaxTotalGBPerRepo 10, got %d", cfg.MaxTotalGBPerRepo)
	}
	if cfg.SizeMarginMB != 0 {
		t.Errorf("Expected SizeMarginMB to be 0, got %d", cfg.SizeMarginMB)
	}
}

func TestConfig_AdvancedFields(t *testing.T) {
	cfg := &Config{
		RepoKeyStyle:    "manual",
		AutoRemoteMerge: true,
		RemoteHashLen:   16,
		NoSize:          true,
	}

	if cfg.RepoKeyStyle != "manual" {
		t.Errorf("Expected RepoKeyStyle 'manual', got %s", cfg.RepoKeyStyle)
	}
	if cfg.AutoRemoteMerge != true {
		t.Errorf("Expected AutoRemoteMerge to be true, got %v", cfg.AutoRemoteMerge)
	}
	if cfg.RemoteHashLen != 16 {
		t.Errorf("Expected RemoteHashLen 16, got %d", cfg.RemoteHashLen)
	}
	if cfg.NoSize != true {
		t.Errorf("Expected NoSize to be true, got %v", cfg.NoSize)
	}
}

func TestBackupResult_DefaultValues(t *testing.T) {
	result := &BackupResult{}

	if result.TotalFiles != 0 {
		t.Errorf("Expected TotalFiles to be 0, got %d", result.TotalFiles)
	}
	if result.CopiedFiles != 0 {
		t.Errorf("Expected CopiedFiles to be 0, got %d", result.CopiedFiles)
	}
	if result.SkippedFiles != 0 {
		t.Errorf("Expected SkippedFiles to be 0, got %d", result.SkippedFiles)
	}
	if result.SkippedDirs != 0 {
		t.Errorf("Expected SkippedDirs to be 0, got %d", result.SkippedDirs)
	}
	if result.PermissionErrs != nil {
		t.Errorf("Expected PermissionErrs to be nil, got %v", result.PermissionErrs)
	}
	if result.OtherErrors != nil {
		t.Errorf("Expected OtherErrors to be nil, got %v", result.OtherErrors)
	}
	if result.PartialSuccess != false {
		t.Errorf("Expected PartialSuccess to be false, got %v", result.PartialSuccess)
	}
}

func TestBackupResult_WithValues(t *testing.T) {
	permissionErrs := []string{"permission denied: /path1", "permission denied: /path2"}
	otherErrors := []string{"disk full", "network error"}

	result := &BackupResult{
		TotalFiles:     100,
		CopiedFiles:    80,
		SkippedFiles:   15,
		SkippedDirs:    5,
		PermissionErrs: permissionErrs,
		OtherErrors:    otherErrors,
		PartialSuccess: true,
	}

	if result.TotalFiles != 100 {
		t.Errorf("Expected TotalFiles 100, got %d", result.TotalFiles)
	}
	if result.CopiedFiles != 80 {
		t.Errorf("Expected CopiedFiles 80, got %d", result.CopiedFiles)
	}
	if result.SkippedFiles != 15 {
		t.Errorf("Expected SkippedFiles 15, got %d", result.SkippedFiles)
	}
	if result.SkippedDirs != 5 {
		t.Errorf("Expected SkippedDirs 5, got %d", result.SkippedDirs)
	}
	if !reflect.DeepEqual(result.PermissionErrs, permissionErrs) {
		t.Errorf("Expected PermissionErrs %v, got %v", permissionErrs, result.PermissionErrs)
	}
	if !reflect.DeepEqual(result.OtherErrors, otherErrors) {
		t.Errorf("Expected OtherErrors %v, got %v", otherErrors, result.OtherErrors)
	}
	if result.PartialSuccess != true {
		t.Errorf("Expected PartialSuccess true, got %v", result.PartialSuccess)
	}
}
