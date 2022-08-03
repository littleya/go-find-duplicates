package service

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/m-manu/go-find-duplicates/bytesutil"
	"github.com/stretchr/testify/assert"
)

// func TestConfig(t *testing.T) {
// 	assert.Equal(t, int64(0), thresholdFileSize%(4*bytesutil.KIBI))
// }

func TestGetDigest(t *testing.T) {
	goRoot := runtime.GOROOT()
	var paths = []string{
		filepath.Join(goRoot, "/src/io/io.go"),
		filepath.Join(goRoot, "/src/io/pipe.go"),
	}
	for _, path := range paths {
		digest, err := GetDigest(path, false, 16*bytesutil.KIBI)
		assert.Equal(t, nil, err)
		assert.Greater(t, digest.FileSize, int64(0))
		assert.Equal(t, 9, len(digest.FileHash))
		assert.Greater(t, len(digest.FileExtension), 0)
	}
	for _, path := range paths {
		digest, err := GetDigest(path, true, 16*bytesutil.KIBI)
		assert.Equal(t, nil, err)
		assert.Greater(t, digest.FileSize, int64(0))
		assert.Equal(t, 64, len(digest.FileHash))
		assert.Greater(t, len(digest.FileExtension), 0)
	}
}
