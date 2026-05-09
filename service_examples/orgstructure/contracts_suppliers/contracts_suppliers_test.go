package contractssuppliers

import (
	"context"
	"strings"
	"testing"
	"time"
)

// ════ STATUS ENUM ════

func TestContractStatus_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		status ContractStatus
		want   bool
	}{
		{"ACTIVE", StatusActive, true},
		{"EXPIRED", StatusExpired, true},
		{"EXPIRING_SOON", StatusExpiringSoon, true},
		{"invalid", ContractStatus("UNKNOWN"), false},
		{"empty", ContractStatus(""), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestComputeStatus(t *testing.T) {
	now := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	timeP := func(t time.Time) *time.Time { return &t }

	tests := []struct {
		name    string
		endDate *time.Time
		want    ContractStatus
	}{
		{"nil end_date is ACTIVE", nil, StatusActive},
		{"end_date in the past is EXPIRED", timeP(now.Add(-24 * time.Hour)), StatusExpired},
		{"end_date == now is EXPIRED", timeP(now), StatusExpired},
		{"end_date within 30 days is EXPIRING_SOON", timeP(now.Add(15 * 24 * time.Hour)), StatusExpiringSoon},
		{"end_date exactly at 30 days is EXPIRING_SOON", timeP(now.Add(30 * 24 * time.Hour)), StatusExpiringSoon},
		{"end_date past 30 days is ACTIVE", timeP(now.Add(31 * 24 * time.Hour)), StatusActive},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := computeStatus(now, tt.endDate); got != tt.want {
				t.Errorf("computeStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ════ ENDPOINT STUBS — TODO ════

func TestValidateCreateRequest(t *testing.T) {
	validDate := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	endAfter := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	endBefore := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)
	timeP := func(t time.Time) *time.Time { return &t }
	tests := []struct {
		name    string
		req     *CreateContractRequest
		wantErr bool
	}{
		{"valid", &CreateContractRequest{ContractNumber: "№123/2025", Amount: 100, SignedDate: validDate}, false},
		{"valid with end_date", &CreateContractRequest{ContractNumber: "№123/2025", Amount: 100, SignedDate: validDate, EndDate: timeP(endAfter)}, false},
		{"zero amount is allowed", &CreateContractRequest{ContractNumber: "№123/2025", Amount: 0, SignedDate: validDate}, false},
		{"nil request", nil, true},
		{"empty contract_number", &CreateContractRequest{ContractNumber: "   ", Amount: 100, SignedDate: validDate}, true},
		{"negative amount", &CreateContractRequest{ContractNumber: "№123/2025", Amount: -0.01, SignedDate: validDate}, true},
		{"zero signed_date", &CreateContractRequest{ContractNumber: "№123/2025", Amount: 100}, true},
		{"end_date before signed_date", &CreateContractRequest{ContractNumber: "№123/2025", Amount: 100, SignedDate: validDate, EndDate: timeP(endBefore)}, true},
		{"end_date equal to signed_date", &CreateContractRequest{ContractNumber: "№123/2025", Amount: 100, SignedDate: validDate, EndDate: timeP(validDate)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCreateRequest(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateCreateRequest() err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestApplyFilterDefaults(t *testing.T) {
	tests := []struct {
		name      string
		page      int
		limit     int
		wantPage  int
		wantLimit int
	}{
		{"zero values use defaults", 0, 0, 1, 20},
		{"negative values use defaults", -5, -10, 1, 20},
		{"valid values pass through", 3, 50, 3, 50},
		{"limit above max is capped", 1, 999, 1, 100},
		{"limit at max stays", 1, 100, 1, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, limit := applyFilterDefaults(tt.page, tt.limit)
			if page != tt.wantPage || limit != tt.wantLimit {
				t.Errorf("applyFilterDefaults(%d, %d) = (%d, %d); want (%d, %d)",
					tt.page, tt.limit, page, limit, tt.wantPage, tt.wantLimit)
			}
		})
	}
}

func TestQueryContractByID_InvalidUUID(t *testing.T) {
	_, err := queryContractByID(context.Background(), "not-a-uuid")
	if err == nil {
		t.Fatal("expected error for invalid UUID, got nil")
	}
	if !strings.Contains(err.Error(), "invalid contract id format") {
		t.Errorf("expected invalid-id error, got: %v", err)
	}
}

func TestValidateUpdateRequest(t *testing.T) {
	str := func(s string) *string { return &s }
	intP := func(i int) *int { return &i }
	floatP := func(f float64) *float64 { return &f }
	timeP := func(t time.Time) *time.Time { return &t }

	validDate := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		req     *UpdateContractRequest
		wantErr bool
	}{
		{"valid single field", &UpdateContractRequest{VatFlag: intP(12)}, false},
		{"valid multiple fields", &UpdateContractRequest{
			ContractNumber: str("№123"),
			SignedDate:     timeP(validDate),
		}, false},
		{"valid amount update", &UpdateContractRequest{
			Amount: floatP(1000),
		}, false},

		// edge cases
		{"nil request", nil, true},
		{"no fields", &UpdateContractRequest{}, true},
		{"empty contract_number", &UpdateContractRequest{ContractNumber: str("   ")}, true},
		{"zero signed_date", &UpdateContractRequest{SignedDate: timeP(time.Time{})}, true},
		{"negative amount", &UpdateContractRequest{Amount: floatP(-10)}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateUpdateRequest(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateUpdateRequest() err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDeleteContract_SoftDelete(t *testing.T) {
	t.Skip("TODO: implement once DeleteContract sets is_active=false")
}

func TestValidateAmendmentRequest(t *testing.T) {
	validDate := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		req     *AmendmentRequest
		wantErr bool
	}{
		{"valid", &AmendmentRequest{AmendmentNumber: "ДС-1", AmendmentDate: validDate, AmendmentAmount: 50}, false},
		{"nil request", nil, true},
		{"empty number", &AmendmentRequest{AmendmentNumber: "  ", AmendmentDate: validDate, AmendmentAmount: 50}, true},
		{"zero date", &AmendmentRequest{AmendmentNumber: "ДС-1", AmendmentAmount: 50}, true},
		{"zero amount", &AmendmentRequest{AmendmentNumber: "ДС-1", AmendmentDate: validDate, AmendmentAmount: 0}, true},
		{"negative amount", &AmendmentRequest{AmendmentNumber: "ДС-1", AmendmentDate: validDate, AmendmentAmount: -10}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAmendmentRequest(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateAmendmentRequest() err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateUploadFileRequest(t *testing.T) {
	smallPDF := []byte("%PDF-1.4\n%data")
	tooBig := make([]byte, 25*1024*1024+1)

	tests := []struct {
		name    string
		req     *UploadFileRequest
		wantErr bool
	}{
		{"valid", &UploadFileRequest{FileName: "contract.pdf", FileData: smallPDF}, false},
		{"nil request", nil, true},
		{"empty file_name", &UploadFileRequest{FileName: "  ", FileData: smallPDF}, true},
		{"empty file_data", &UploadFileRequest{FileName: "contract.pdf", FileData: nil}, true},
		{"too large", &UploadFileRequest{FileName: "contract.pdf", FileData: tooBig}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateUploadFileRequest(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateUploadFileRequest() err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIsAllowedMimeType(t *testing.T) {
	cases := map[string]bool{
		"application/pdf":  true,
		"image/png":        true,
		"image/jpeg":       true,
		"image/gif":        false,
		"text/plain":       false,
		"application/json": false,
		"":                 false,
	}
	for mime, want := range cases {
		if got := isAllowedMimeType(mime); got != want {
			t.Errorf("isAllowedMimeType(%q) = %v, want %v", mime, got, want)
		}
	}
}

func TestBuildFileKey(t *testing.T) {
	cases := []struct {
		contractID, fileName, want string
	}{
		{"abc123", "contract.pdf", "abc123/contract.pdf"},
		{"abc123", "  contract.pdf  ", "abc123/contract.pdf"},
		{"abc123", "/etc/passwd", "abc123/passwd"}, // strips directory traversal
		{"abc123", "../../secret.pdf", "abc123/secret.pdf"},
	}
	for _, c := range cases {
		if got := buildFileKey(c.contractID, c.fileName); got != c.want {
			t.Errorf("buildFileKey(%q, %q) = %q, want %q", c.contractID, c.fileName, got, c.want)
		}
	}
}

func TestImportContracts(t *testing.T) {
	t.Skip("TODO: implement once ImportContracts parses CSV/XLSX")
}
