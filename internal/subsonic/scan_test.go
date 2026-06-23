package subsonic

import "testing"

func TestGetScanStatusIdle(t *testing.T) {
	h, _ := setupSubsonic(t)
	resp := doGet(h, testAPIKey, "getScanStatus", "")
	if resp["status"] != "ok" {
		t.Fatalf("status=%v, want ok", resp["status"])
	}
	ss, _ := resp["scanStatus"].(map[string]any)
	if ss == nil {
		t.Fatalf("no scanStatus: %v", resp)
	}
	if ss["scanning"] != false {
		t.Errorf("scanning=%v, want false when idle", ss["scanning"])
	}
}

func TestStartScanNoFolder(t *testing.T) {
	// setupSubsonic enables music with NO folder_node_id, so startScan has nothing to scan.
	h, _ := setupSubsonic(t)
	resp := doGet(h, testAPIKey, "startScan", "")
	if resp["status"] != "ok" {
		t.Errorf("status=%v, want ok", resp["status"])
	}
	ss, _ := resp["scanStatus"].(map[string]any)
	if ss == nil {
		t.Fatalf("no scanStatus: %v", resp)
	}
}
