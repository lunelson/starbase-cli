package main

import (
	"bytes"
	"testing"
)

func TestRootCommand(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantOut string
		wantErr bool
	}{
		{
			name:    "version flag",
			args:    []string{"--version"},
			wantOut: "starbase version",
		},
		{
			name:    "help flag",
			args:    []string{"--help"},
			wantOut: "searchable local mirror",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := new(bytes.Buffer)
			rootCmd.SetOut(buf)
			rootCmd.SetErr(buf)
			rootCmd.SetArgs(tt.args)

			err := rootCmd.Execute()
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantOut != "" && !bytes.Contains(buf.Bytes(), []byte(tt.wantOut)) {
				t.Errorf("Output = %q, want contains %q", buf.String(), tt.wantOut)
			}
		})
	}
}
