package notebook

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadYamlFile(t *testing.T) {
	type TestStruct struct {
		Name  string `yaml:"name"`
		Value int    `yaml:"value"`
	}

	tests := []struct {
		name             string
		fileContent      string
		filePath         string
		createFile       bool
		expected         TestStruct
		expectError      bool
		wantErrorContain string
	}{
		{
			name: "valid yaml",
			fileContent: `name: test
value: 42`,
			createFile: true,
			expected: TestStruct{
				Name:  "test",
				Value: 42,
			},
		},
		{
			name:        "empty file",
			fileContent: "",
			createFile:  true,
			expected:    TestStruct{},
			expectError: true,
		},
		{
			name: "yaml with extra fields",
			fileContent: `name: test
value: 42
extra: ignored`,
			createFile: true,
			expected: TestStruct{
				Name:  "test",
				Value: 42,
			},
		},
		{
			name: "invalid yaml",
			fileContent: `name: test
value: [invalid`,
			createFile:  true,
			expected:    TestStruct{},
			expectError: true,
		},
		{
			name:             "nonexistent file",
			filePath:         "/nonexistent/file.yml",
			createFile:       false,
			expected:         TestStruct{},
			expectError:      true,
			wantErrorContain: "os.Open",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var filePath string

			if tt.createFile {
				tempDir, err := os.MkdirTemp("", "readyaml_test")
				require.NoError(t, err)
				defer func() {
					_ = os.RemoveAll(tempDir)
				}()

				filePath = filepath.Join(tempDir, "test.yml")
				err = os.WriteFile(filePath, []byte(tt.fileContent), 0644)
				require.NoError(t, err)
			} else {
				filePath = tt.filePath
			}

			result, err := readYamlFile[TestStruct](filePath)

			if tt.expectError {
				assert.Error(t, err)
				if tt.wantErrorContain != "" {
					assert.Contains(t, err.Error(), tt.wantErrorContain)
				}
				assert.Equal(t, tt.expected, result)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWriteYamlFile(t *testing.T) {
	type TestStruct struct {
		Name  string `yaml:"name"`
		Value int    `yaml:"value"`
	}

	tests := []struct {
		name             string
		data             TestStruct
		filePath         string
		useTempDir       bool
		expected         string
		expectError      bool
		wantErrorContain string
	}{
		{
			name: "simple struct",
			data: TestStruct{
				Name:  "test",
				Value: 42,
			},
			useTempDir: true,
			expected:   "name: test\nvalue: 42\n",
		},
		{
			name:       "empty struct",
			data:       TestStruct{},
			useTempDir: true,
			expected:   "name: \"\"\nvalue: 0\n",
		},
		{
			name: "struct with special characters",
			data: TestStruct{
				Name:  "test with spaces & symbols",
				Value: -1,
			},
			useTempDir: true,
			expected:   "name: test with spaces & symbols\nvalue: -1\n",
		},
		{
			name: "invalid path",
			data: TestStruct{
				Name: "test",
			},
			filePath:         "/nonexistent/directory/file.yml",
			useTempDir:       false,
			expectError:      true,
			wantErrorContain: "os.Create",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var filePath string

			if tt.useTempDir {
				tempDir, err := os.MkdirTemp("", "writeyaml_test")
				require.NoError(t, err)
				defer func() {
					_ = os.RemoveAll(tempDir)
				}()

				filePath = filepath.Join(tempDir, "test.yml")
			} else {
				filePath = tt.filePath
			}

			err := WriteYamlFile(filePath, tt.data)

			if tt.expectError {
				assert.Error(t, err)
				if tt.wantErrorContain != "" {
					assert.Contains(t, err.Error(), tt.wantErrorContain)
				}
				return
			}

			assert.NoError(t, err)

			// Verify the file was created and has correct content
			content, err := os.ReadFile(filePath)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, string(content))

			// Verify we can read it back
			result, err := readYamlFile[TestStruct](filePath)
			assert.NoError(t, err)
			assert.Equal(t, tt.data, result)
		})
	}
}
