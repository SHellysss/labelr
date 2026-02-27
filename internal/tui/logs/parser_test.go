package logs

import "testing"

func TestParseLine(t *testing.T) {
	tests := []struct {
		input string
		want  LogEntry
	}{
		{
			input: "2026/02/27 14:23:01 INFO  Polling for new emails",
			want:  LogEntry{Time: "14:23:01", Level: "INFO", Message: "Polling for new emails"},
		},
		{
			input: "2026/02/27 14:23:04 ERROR Failed to classify message",
			want:  LogEntry{Time: "14:23:04", Level: "ERROR", Message: "Failed to classify message"},
		},
		{
			input: "2026/02/27 14:23:05 DEBUG Detailed debug info here",
			want:  LogEntry{Time: "14:23:05", Level: "DEBUG", Message: "Detailed debug info here"},
		},
		{
			input: "2026/02/27 14:23:06 WARN Rate limited, retrying",
			want:  LogEntry{Time: "14:23:06", Level: "WARN", Message: "Rate limited, retrying"},
		},
		{
			input: "some unstructured line",
			want:  LogEntry{Time: "", Level: "", Message: "some unstructured line"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseLine(tt.input)
			if got.Time != tt.want.Time {
				t.Errorf("Time: got %q, want %q", got.Time, tt.want.Time)
			}
			if got.Level != tt.want.Level {
				t.Errorf("Level: got %q, want %q", got.Level, tt.want.Level)
			}
			if got.Message != tt.want.Message {
				t.Errorf("Message: got %q, want %q", got.Message, tt.want.Message)
			}
		})
	}
}
