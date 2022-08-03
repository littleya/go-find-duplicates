package entity

import "fmt"

// FileExtAndSize is a struct of file extension and file size
type FileExtAndSize struct {
	// FileExtension string
	FileSize int64
}

func (f FileExtAndSize) String() string {
	// return fmt.Sprintf("%v/%v", f.FileExtension, f.FileSize)
	return fmt.Sprintf("%v", f.FileSize)
}

// FileExtAndSizeToFiles is a multi-map of FileExtAndSize key and string values
type FileExtAndSizeToFiles map[FileExtAndSize][]string
