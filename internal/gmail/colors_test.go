package gmail

import "testing"

func TestColorForLabel_DefaultLabels(t *testing.T) {
	tests := []struct {
		name   string
		wantBg string
		wantTx string
	}{
		{"Action Required", "#cc3a21", "#ffffff"},
		{"Informational", "#4a86e8", "#ffffff"},
		{"Newsletter", "#fad165", "#000000"},
		{"Finance", "#16a766", "#ffffff"},
		{"Scheduling", "#a479e2", "#ffffff"},
		{"Personal", "#f691b3", "#000000"},
		{"Automated", "#999999", "#ffffff"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bg, tx := ColorForLabel(tt.name, 0)
			if bg != tt.wantBg {
				t.Errorf("bg: got %q, want %q", bg, tt.wantBg)
			}
			if tx != tt.wantTx {
				t.Errorf("text: got %q, want %q", tx, tt.wantTx)
			}
		})
	}
}

func TestColorForLabel_CustomLabels(t *testing.T) {
	// Custom label at index 0 should get first overflow color
	bg0, tx0 := ColorForLabel("My Custom", 0)
	if bg0 != "#ffad47" || tx0 != "#000000" {
		t.Errorf("index 0: got %q/%q, want #ffad47/#000000", bg0, tx0)
	}

	// Custom label at index 1 should get second overflow color
	bg1, tx1 := ColorForLabel("Another Custom", 1)
	if bg1 != "#3c78d8" || tx1 != "#ffffff" {
		t.Errorf("index 1: got %q/%q, want #3c78d8/#ffffff", bg1, tx1)
	}
}

func TestIsDefaultLabel(t *testing.T) {
	if !IsDefaultLabel("Action Required") {
		t.Error("expected Action Required to be a default label")
	}
	if IsDefaultLabel("My Custom Label") {
		t.Error("expected My Custom Label to not be a default label")
	}
}

func TestColorForLabel_OverflowWraps(t *testing.T) {
	// Index beyond palette length should wrap around
	bg0, _ := ColorForLabel("Wrap Test", 0)
	bg8, _ := ColorForLabel("Wrap Test 2", 8)
	if bg0 != bg8 {
		t.Errorf("expected wrap: index 0 bg=%q, index 8 bg=%q", bg0, bg8)
	}
}
