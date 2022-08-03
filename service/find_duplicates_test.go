package service

import (
	"fmt"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/m-manu/go-find-duplicates/bytesutil"
	"github.com/m-manu/go-find-duplicates/utils"
	"github.com/stretchr/testify/assert"
)

const exclusionsStr = `.DS_Store
vendor
`

func TestFindDuplicates(t *testing.T) {
	goRoot := runtime.GOROOT()
	directories := []string{
		filepath.Join(goRoot, "pkg"),
		filepath.Join(goRoot, "src"),
		filepath.Join(goRoot, "test"),
	}
	exclusions, _ := utils.LineSeparatedStrToMap(exclusionsStr)
	duplicates, duplicateCount, savingsSize, _, err := FindDuplicates(directories, exclusions,
		4_196, 2, false, 16*bytesutil.KIBI)
	assert.Nil(t, err)
	assert.GreaterOrEqual(t, duplicates.Size(), 0)
	assert.GreaterOrEqual(t, duplicateCount, int64(0))
	assert.GreaterOrEqual(t, savingsSize, int64(0))
}

func TestNonThoroughVsNot(t *testing.T) {
	exclusions, _ := utils.LineSeparatedStrToMap(exclusionsStr)
	goRoot := []string{runtime.GOROOT()}
	fmt.Printf("*** Scanning %s with 'thorough mode' on ***\n", goRoot)
	_, duplicateCountExpected, savingsSizeExpected, _, tErr := FindDuplicates(goRoot, exclusions,
		4_196, 2, true, 16*bytesutil.KIBI)
	assert.Nil(t, tErr, "error while scanning for duplicates in GOROOT directory")
	fmt.Printf("*** Scanning %s with 'thorough mode' off ***\n", goRoot)
	_, duplicateCountActual, savingsSizeActual, _, ntErr := FindDuplicates(goRoot, exclusions,
		4_196, 5, false, 16*bytesutil.KIBI)
	assert.Nil(t, ntErr, "error while thoroughly scanning for duplicates in GOROOT directory")
	assert.Equal(t, duplicateCountExpected, duplicateCountActual)
	assert.Equal(t, savingsSizeExpected, savingsSizeActual)
}
