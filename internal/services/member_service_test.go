package services

import "testing"

func TestNormalizeRejectionReasons(t *testing.T) {
	reasons := normalizeRejectionReasons([]string{
		" รูปหน้าร้านไม่ชัดเจน ",
		"",
		"ลิงก์ร้านเปิดไม่ได้",
		"รูปหน้าร้านไม่ชัดเจน",
	})

	if len(reasons) != 2 {
		t.Fatalf("expected 2 normalized reasons, got %d", len(reasons))
	}
	if reasons[0] != "รูปหน้าร้านไม่ชัดเจน" {
		t.Fatalf("unexpected first reason: %q", reasons[0])
	}
	if reasons[1] != "ลิงก์ร้านเปิดไม่ได้" {
		t.Fatalf("unexpected second reason: %q", reasons[1])
	}
}

func TestAllowedMemberStatus(t *testing.T) {
	for _, status := range []string{"pending", "approved", "rejected"} {
		if !allowedMemberStatus(status) {
			t.Fatalf("expected %q to be allowed", status)
		}
	}

	if allowedMemberStatus("deleted") {
		t.Fatal("expected deleted status to be rejected")
	}
}
