package util

import "os"

type FileHandler interface {
	Write(path string, data []byte, perm os.FileMode) error
	CreateDir(path string, perm os.FileMode) error
	GetHomeDir() (string, error)
}

type OSFileHandler struct{}

func (h *OSFileHandler) GetHomeDir() (string, error) {
	return os.UserHomeDir()
}

func (h *OSFileHandler) CreateDir(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (h *OSFileHandler) Write(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}
