package cmd

import "testing"

func TestParseNewArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		want    newOptions
		wantErr string
	}{
		{
			name: "project only",
			args: []string{"srv-project"},
			want: newOptions{projectName: "srv-project"},
		},
		{
			name: "flags after project",
			args: []string{"srv-project", "--module", "github.com/acme/srv-project", "--service-name", "srv-custom", "--router-path", "/custom"},
			want: newOptions{
				projectName: "srv-project",
				moduleName:  "github.com/acme/srv-project",
				serviceName: "srv-custom",
				routerPath:  "/custom",
			},
		},
		{
			name: "flags before project with equals",
			args: []string{"--module=github.com/acme/srv-project", "--service-name=srv-custom", "--router-path=custom", "srv-project"},
			want: newOptions{
				projectName: "srv-project",
				moduleName:  "github.com/acme/srv-project",
				serviceName: "srv-custom",
				routerPath:  "custom",
			},
		},
		{
			name:    "missing project",
			args:    []string{"--module", "github.com/acme/srv-project"},
			wantErr: "usage: easygin new <project> [--module <module>] [--service-name <name>] [--router-path <path>]",
		},
		{
			name:    "missing flag value",
			args:    []string{"srv-project", "--module"},
			wantErr: "flag --module requires a value",
		},
		{
			name:    "unknown flag",
			args:    []string{"srv-project", "--unknown", "x"},
			wantErr: `unknown flag "--unknown"`,
		},
		{
			name:    "duplicate positional argument",
			args:    []string{"srv-project", "srv-other"},
			wantErr: `unexpected argument "srv-other"`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseNewArgs(tt.args)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("error = %q, want %q", err.Error(), tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("parseNewArgs returned error: %v", err)
			}

			if got != tt.want {
				t.Fatalf("parseNewArgs(%v) = %+v, want %+v", tt.args, got, tt.want)
			}
		})
	}
}
