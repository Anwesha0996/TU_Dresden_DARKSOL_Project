package helmdd

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// MockFileHandler implements FileHandler for testing purposes.
type MockFileHandler struct {
	// Inputs to simulate
	HomeDir         string
	WriteError      error
	CreateDirError  error
	GetHomeDirError error

	// Outputs to capture
	WrittenFilePath    string
	WrittenFileContent []byte
	CreatedDirPath     string
}

func (m *MockFileHandler) GetHomeDir() (string, error) {
	return m.HomeDir, m.GetHomeDirError
}

func (m *MockFileHandler) CreateDir(path string, perm os.FileMode) error {
	m.CreatedDirPath = path // Capture the path
	return m.CreateDirError
}

func (m *MockFileHandler) Write(path string, data []byte, perm os.FileMode) error {
	m.WrittenFilePath = path    // Capture the path
	m.WrittenFileContent = data // Capture the content
	return m.WriteError
}

func TestResolveDependencies(t *testing.T) {
	if _, err := ResolveDependencies("testdata/example", ""); err != nil {
		t.Errorf(`ResolveDependencies("example") = %v`, err)
	}
}

func TestResolveDependenciesEmpty(t *testing.T) {
	if _, err := ResolveDependencies("", ""); err == nil {
		t.Errorf(`ResolveDependencies("") = nil, expected: 'stat : no such file or directory'`)
	}
}

func TestRenderTemplate(t *testing.T) {
	manifest, err := renderTemplate("testdata/simple_example")

	if err != nil {
		t.Errorf(`renderTemplate("simple_example") = nil, %v, expected manifest, nil`, err)
	}

	testManifest, _ := os.ReadFile("testdata/test_manifests.yaml")

	if manifest != strings.TrimRight(string(testManifest), "\n") {
		t.Errorf(`renderTemplate("simpleexample") = got %q, expected %q`, manifest, strings.TrimRight(string(testManifest), "\n"))

	}
}

func TestRenderTemplateEmpty(t *testing.T) {
	if _, err := renderTemplate(""); err == nil {
		t.Error(`renderTemplate("") = manifest, nil, expected nil, 'stat : no such file or directory'`)
	}
}

func TestExportDependencyListYAML(t *testing.T) {
	const expectedOutput = `git:
    https://gitlab.com/d3tn/ud3tn.git: /var/www/html/assets
images:
    alpine:
        - git:latest
    myregistry:
        - web-app-api:v1.0.0`

	testDependencies := map[string]any{"git": map[string]string{"https://gitlab.com/d3tn/ud3tn.git": "/var/www/html/assets"}, "images": map[string][]string{"alpine": []string{"git:latest"}, "myregistry": []string{"web-app-api:v1.0.0"}}}
	mockHandler := &MockFileHandler{
		HomeDir:         "/home/testuser", // Simulate a home directory
		WriteError:      nil,              // No errors
		CreateDirError:  nil,
		GetHomeDirError: nil,
	}

	_, err := exportDependencyListYAML(testDependencies, mockHandler)

	// Check for general error
	if err != nil {
		t.Fatalf("exportDependencyListYAML(testDependencies, mockHandler) = %v, expected nil error", err)
	}

	// Check if the correct directory was targeted
	expectedDirPath := "/home/testuser/.local/share/helm-dtn"
	if mockHandler.CreatedDirPath != expectedDirPath {
		t.Errorf("exportDependencyListYAML(testDependencies, mockHandler): CreatedDirPath mismatch. Got: %s, Expected: %s, ", mockHandler.CreatedDirPath, expectedDirPath)
	}

	// Check if the correct file path was targeted
	expectedFilePath := "/home/testuser/.local/share/helm-dtn/helmdd.yaml"
	if mockHandler.WrittenFilePath != expectedFilePath {
		t.Errorf("exportDependencyListYAML(inputData, mockHandler): WrittenFilePath mismatch. Expected: %s, Got: %s", expectedFilePath, mockHandler.WrittenFilePath)
	}

	// Check if the content is correct YAML
	if strings.TrimRight(string(mockHandler.WrittenFileContent), "\n") != expectedOutput {
		t.Errorf("exportDependencyListYAML(testDependencies, mockHandler) = got %q, expected %q`,", string(mockHandler.WrittenFileContent), expectedOutput)
	}
}

func TestExportDataListYAMLGetHomeDirFailure(t *testing.T) {
	// We're testing for an error, so the data does not matter
	inputData := map[string]any{"key": "value"}

	// Inject the error into the GetHomeDir call
	expectedErr := errors.New("failed to retrieve user home directory")
	mockHandler := &MockFileHandler{
		GetHomeDirError: expectedErr,
	}

	_, err := exportDependencyListYAML(inputData, mockHandler)

	if !errors.Is(err, expectedErr) {
		t.Errorf("exportDependencyListYAML(inputData, mockHandler) = %v, expected %v", err, expectedErr)
	}
	// Verify that there were no file/directory modifications
	if len(mockHandler.CreatedDirPath) > 0 || len(mockHandler.WrittenFileContent) > 0 {
		t.Error("exportDependencyListYAML(inputData, mockHandler): File operations should not have been called after home directory failure")
	}
}

func TestExportDataListYAMLMkDirFailure(t *testing.T) {
	inputData := map[string]any{"key": "value"}

	expectedErr := errors.New("failed to create directory")
	mockHandler := &MockFileHandler{
		HomeDir:        "/home/testuser",
		CreateDirError: expectedErr,
	}

	_, err := exportDependencyListYAML(inputData, mockHandler)

	if !errors.Is(err, expectedErr) {
		t.Errorf("exportDependencyListYAML(inputData, mockHandler) = %v, expected %v", err, expectedErr)
	}

	expectedPath := "/home/testuser/.local/share/helm-dtn"

	if mockHandler.CreatedDirPath != expectedPath {
		t.Errorf("exportDependencyListYAML(inputData, mockHandler): Expected CreateDir to be called on path: %s, got: %s", expectedPath, mockHandler.CreatedDirPath)
	}

	if len(mockHandler.WrittenFileContent) > 0 {
		t.Error("exportDependencyListYAML(inputData, mockHandler): File writing should not have been called after directory creation failure")
	}

}

func TestExportDataListYAMLWriteFailure(t *testing.T) {
	inputData := map[string]any{"key": "value"}

	expectedErr := errors.New("disk is full")
	mockHandler := &MockFileHandler{
		HomeDir:    "/home/testuser",
		WriteError: expectedErr,
	}

	_, err := exportDependencyListYAML(inputData, mockHandler)

	if !errors.Is(err, expectedErr) {
		t.Errorf("exportDependencyListYAML(inputData, mockHandler) = %v, expected %v", err, expectedErr)
	}
}
