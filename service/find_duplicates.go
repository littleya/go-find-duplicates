package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/m-manu/go-find-duplicates/bytesutil"
	"github.com/m-manu/go-find-duplicates/entity"
	"github.com/m-manu/go-find-duplicates/fmte"
)

// FindDuplicates finds duplicate files in a given set of directories and matching criteria
func FindDuplicates(directories []string, excludedFiles map[string]struct{}, fileSizeThreshold int64, parallelism int,
	isThorough bool, hashSize int64) (
	duplicates *entity.DigestToFiles, duplicateTotalCount int64, savingsSize int64,
	allFiles entity.FilePathToMeta, err error,
) {
	fmte.Printf("Scanning %d directories...\n", len(directories))
	allFiles = make(entity.FilePathToMeta, 10_000)
	var totalSize int64
	for _, dirPath := range directories {
		size, pErr := populateFilesFromDirectory(dirPath, excludedFiles, fileSizeThreshold, allFiles)
		if pErr != nil {
			err = fmt.Errorf("error while scaning directory %s: %+v", dirPath, pErr)
			return
		}
		totalSize += size
	}
	fmte.Printf("Done. Found %d files of total size %s.\n", len(allFiles), bytesutil.BinaryFormat(totalSize))
	if len(allFiles) == 0 {
		return
	}
	fmte.Printf("Finding potential duplicates... \n")
	shortlist := identifyShortList(allFiles)
	if len(shortlist) == 0 {
		return
	}
	fmte.Printf("Completed. Found %d files that may have one or more duplicates!\n", len(shortlist))
	if isThorough {
		fmte.Printf("Thoroughly scanning for duplicates... \n")
	} else {
		fmte.Printf("Scanning for duplicates... \n")
	}
	var processedCount int32
	var wg sync.WaitGroup
	wg.Add(2)
	go func(pc *int32, fc int32) {
		defer wg.Done()
		time.Sleep(200 * time.Millisecond)
		for atomic.LoadInt32(pc) < fc {
			time.Sleep(2 * time.Second)
			progress := float64(atomic.LoadInt32(pc)) / float64(fc)
			fmte.Printf("%2.0f%% processed so far\n", progress*100.0)
		}
	}(&processedCount, int32(len(shortlist)))
	go func(p *int32) {
		defer wg.Done()
		duplicates = entity.NewDigestToFiles()
		computeDigestsAndGroupThem(shortlist, parallelism, p, duplicates, isThorough, hashSize)
		for iter := duplicates.Iterator(); iter.HasNext(); {
			digest, files := iter.Next()
			numDuplicates := int64(len(files)) - 1
			duplicateTotalCount += numDuplicates
			savingsSize += numDuplicates * digest.FileSize
		}
	}(&processedCount)
	wg.Wait()
	fmte.Printf("Scan completed.\n")
	return
}

func computeDigestsAndGroupThem(shortlist entity.FileExtAndSizeToFiles, parallelism int,
	processedCount *int32, duplicates *entity.DigestToFiles, isThorough bool, hashSize int64,
) {
	// Find potential duplicates:
	slKeys := make([]entity.FileExtAndSize, 0, len(shortlist))
	for extAndSize := range shortlist {
		slKeys = append(slKeys, extAndSize)
	}
	var wg sync.WaitGroup
	wg.Add(parallelism)
	for i := 0; i < parallelism; i++ {
		go func(shard int, wg *sync.WaitGroup, count *int32) {
			defer wg.Done()
			low := shard * len(slKeys) / parallelism
			high := (shard + 1) * len(slKeys) / parallelism
			for _, fileExtAndSize := range slKeys[low:high] {
				for _, path := range shortlist[fileExtAndSize] {
					digest, err := GetDigest(path, isThorough, hashSize)
					if err != nil {
						fmte.Printf("error while scanning %s: %+v\n", path, err)
						continue
					}
					duplicates.Set(digest, path)
				}
				atomic.AddInt32(count, 1)
			}
		}(i, &wg, processedCount)
	}
	wg.Wait()
	// Remove non-duplicates
	for iter := duplicates.Iterator(); iter.HasNext(); {
		digest, files := iter.Next()
		if len(files) <= 1 {
			duplicates.Remove(digest)
		}
	}
	return
}

func identifyShortList(filesAndMeta entity.FilePathToMeta) (shortlist entity.FileExtAndSizeToFiles) {
	shortlist = make(entity.FileExtAndSizeToFiles, len(filesAndMeta))
	// Group the files that have same extension and same size
	for path, meta := range filesAndMeta {
		// fileExtAndSize := entity.FileExtAndSize{FileExtension: utils.GetFileExt(path), FileSize: meta.Size}
		fileExtAndSize := entity.FileExtAndSize{FileSize: meta.Size}
		shortlist[fileExtAndSize] = append(shortlist[fileExtAndSize], path)
	}
	// Remove non-duplicates
	for fileExtAndSize, paths := range shortlist {
		if len(paths) <= 1 {
			delete(shortlist, fileExtAndSize)
		}
	}
	return shortlist
}

func RecheckDuplicates(duplicates *entity.DigestToFiles, parallelism int) *entity.DigestToFiles {
	dup := entity.NewDigestToFiles()

	handleCount := 0
	pathCount := 0
	dup_map := map[*entity.FileDigest][]string{}
	for iter := duplicates.Iterator(); iter.HasNext(); {
		digest, paths := iter.Next()
		dup_map[digest] = paths
		pathCount += len(paths)
	}

	var wg sync.WaitGroup
	wg.Add(parallelism + 1)
	lock := sync.Mutex{}
	go func(pc *int, fc int) {
		defer wg.Done()
		time.Sleep(200 * time.Millisecond)
		for *pc < fc {
			time.Sleep(2 * time.Second)
			progress := float64(*pc) / float64(fc)
			fmte.Printf("%d/%d, %2.0f%% processed so far\n", *pc, fc, progress*100.0)
		}
	}(&handleCount, pathCount)
	for i := 0; i < parallelism; i++ {
		go func(wg *sync.WaitGroup, lock *sync.Mutex) {
			defer wg.Done()
			for {
				lock.Lock()
				var digest *entity.FileDigest
				var paths []string
				for k, v := range dup_map {
					digest = k
					paths = v
				}
				delete(dup_map, digest)
				if len(paths) == 0 {
					lock.Unlock()
					break
				}
				lock.Unlock()
				// map from hash to paths
				fileHash := map[string][]string{}
				for _, path := range paths {
					h, _ := checksum(path)
					lock.Lock()
					handleCount += 1
					lock.Unlock()
					if _, ok := fileHash[h]; ok {
						fileHash[h] = append(fileHash[h], path)
					} else {
						fileHash[h] = []string{path}
					}
				}
				// build new dup
				for hashString, paths := range fileHash {
					if len(paths) > 1 {
						for _, path := range paths {
							dig := entity.FileDigest{
								FileExtension: digest.FileExtension,
								FileSize:      digest.FileSize,
								FileHash:      hashString,
							}
							dup.Set(dig, path)
						}
					}
				}
			}
		}(&wg, &lock)
	}
	wg.Wait()
	// duplicates = dup
	return dup
}

func checksum(file string) (string, error) {
	f, err := os.Open(file)
	if err != nil {
		return "", err
	}

	defer func() {
		_ = f.Close()
	}()

	copyBuf := make([]byte, 1024*1024)

	h := sha256.New()
	if _, err := io.CopyBuffer(h, f, copyBuf); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
