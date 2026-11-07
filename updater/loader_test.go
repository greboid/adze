package updater

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

const loaderTestCompose = `services:
  app:
    image: example/app
    environment:
      DB_URL: postgres://app:${ADZE_TEST_DB_PASSWORD}@db/app
`

func TestComposeProjectLoader_DotEnv(t *testing.T) {
	tests := []struct {
		name        string
		dotEnv      string
		osEnv       string
		configFiles func(dir string) []string
		expected    string
	}{
		{
			name:   "explicit config file",
			dotEnv: "ADZE_TEST_DB_PASSWORD=fromdotenv\n",
			configFiles: func(dir string) []string {
				return []string{filepath.Join(dir, "docker-compose.yml")}
			},
			expected: "postgres://app:fromdotenv@db/app",
		},
		{
			name:     "discovered config file",
			dotEnv:   "ADZE_TEST_DB_PASSWORD=fromdotenv\n",
			expected: "postgres://app:fromdotenv@db/app",
		},
		{
			name:     "env file wins over os environment",
			dotEnv:   "ADZE_TEST_DB_PASSWORD=fromdotenv\n",
			osEnv:    "fromos",
			expected: "postgres://app:fromdotenv@db/app",
		},
		{
			name:     "os environment is not passed through",
			osEnv:    "fromos",
			expected: "postgres://app:@db/app",
		},
		{
			name:     "no env file",
			expected: "postgres://app:@db/app",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(loaderTestCompose), 0o644); err != nil {
				t.Fatal(err)
			}
			if tt.dotEnv != "" {
				if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(tt.dotEnv), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if tt.osEnv != "" {
				t.Setenv("ADZE_TEST_DB_PASSWORD", tt.osEnv)
			}

			var configFiles []string
			if tt.configFiles != nil {
				configFiles = tt.configFiles(dir)
			}

			project, err := ComposeProjectLoader{}.LoadProject(context.Background(), dir, configFiles)
			if err != nil {
				t.Fatalf("LoadProject: %v", err)
			}

			got := project.Services["app"].Environment["DB_URL"]
			if got == nil {
				t.Fatal("DB_URL not set on service")
			}
			if *got != tt.expected {
				t.Errorf("DB_URL = %q, want %q", *got, tt.expected)
			}
		})
	}
}
