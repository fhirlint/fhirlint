package validator

import (
	"testing"
)

func TestParseJavaMajorVersion(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		want    int
		wantErr bool
	}{
		{
			name:   "Java 8 legacy format",
			output: `java version "1.8.0_392"`,
			want:   8,
		},
		{
			name:   "Java 11",
			output: `openjdk version "11.0.23" 2024-04-16`,
			want:   11,
		},
		{
			name:   "Java 17",
			output: `openjdk version "17.0.11" 2024-04-16`,
			want:   17,
		},
		{
			name:   "Java 21",
			output: `openjdk version "21.0.3" 2024-04-16`,
			want:   21,
		},
		{
			name:    "unrecognized output",
			output:  `something unexpected`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseJavaMajorVersion(tt.output)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseJavaMajorVersion() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseJavaMajorVersion() = %d, want %d", got, tt.want)
			}
		})
	}
}
