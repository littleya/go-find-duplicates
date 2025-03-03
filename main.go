/*
A blazingly-fast simple-to-use tool to find duplicate files (photos, videos, music, documents etc.) on your computer,
portable hard drives etc.
*/
package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/m-manu/go-find-duplicates/bytesutil"
	"github.com/m-manu/go-find-duplicates/entity"
	"github.com/m-manu/go-find-duplicates/fmte"
	"github.com/m-manu/go-find-duplicates/service"
	"github.com/m-manu/go-find-duplicates/utils"
	flag "github.com/spf13/pflag"
)

// Exit codes for this program
const (
	exitCodeSuccess = iota
	exitCodeInvalidNumArgs
	exitCodeInvalidExclusions
	exitCodeInputDirectoryNotReadable
	exitCodeExclusionFilesError
	exitCodeErrorFindingDuplicates
	exitCodeErrorCreatingReport
	exitCodeInvalidOutputMode
	exitCodeReportFileCreationFailed
	exitCodeWritingToReportFileFailed
)

//go:embed default_exclusions.txt
var defaultExclusionsStr string

var flags struct {
	isHelp           func() bool
	getOutputMode    func() string
	getExcludedFiles func() map[string]struct{}
	getMinSize       func() int64
	getParallelism   func() int
	isThorough       func() bool
	getHashSize      func() int64
}

func setupExclusionsOpt() {
	const exclusionsFlag = "exclusions"
	const exclusionsDefaultValue = ""
	defaultExclusions, defaultExclusionsExamples := utils.LineSeparatedStrToMap(defaultExclusionsStr)
	excludesListFilePathPtr := flag.StringP(exclusionsFlag, "x", exclusionsDefaultValue,
		fmt.Sprintf("path to file containing newline-separated list of file/directory names to be excluded\n"+
			"(if this is not set, by default these will be ignored:\n%s etc.)",
			strings.Join(defaultExclusionsExamples, ", ")))
	flags.getExcludedFiles = func() map[string]struct{} {
		excludesListFilePath := *excludesListFilePathPtr
		var exclusions map[string]struct{}
		if excludesListFilePath == exclusionsDefaultValue {
			exclusions = defaultExclusions
		} else {
			if !utils.IsReadableFile(excludesListFilePath) {
				fmte.PrintfErr("error: argument to flag --%s should be a readable file\n", exclusionsFlag)
				flag.Usage()
				os.Exit(exitCodeInvalidExclusions)
			}
			rawContents, err := os.ReadFile(excludesListFilePath)
			if err != nil {
				fmte.PrintfErr("error: unable to read exclusions file: %+v\n", exclusionsFlag, err)
				flag.Usage()
				os.Exit(exitCodeExclusionFilesError)
			}
			contents := strings.ReplaceAll(string(rawContents), "\r\n", "\n") // Windows
			exclusions, _ = utils.LineSeparatedStrToMap(contents)
		}
		return exclusions
	}
}

func setupHelpOpt() {
	helpPtr := flag.BoolP("help", "h", false, "display help")
	flags.isHelp = func() bool {
		return *helpPtr
	}
}

func setupThoroughOpt() {
	thoroughPtr := flag.BoolP("thorough", "t", false,
		"apply thorough check of uniqueness of files\n(caution: this makes the scan very slow!)",
	)
	flags.isThorough = func() bool {
		return *thoroughPtr
	}
}

func setupMinSizeOpt() {
	fileSizeThresholdPtr := flag.Uint64P("minsize", "m", 4,
		"minimum size of file in KiB to consider",
	)
	flags.getMinSize = func() int64 {
		return int64(*fileSizeThresholdPtr) * bytesutil.KIBI
	}
}

func setupHashSizeOpt() {
	hashSizePtr := flag.Int64P("hashsize", "s", 16, "The size which used to hash")
	flags.getHashSize = func() int64 {
		return int64(*hashSizePtr) * bytesutil.KIBI
	}
}

func setupParallelismOpt() {
	const defaultParallelismValue = 0
	parallelismPtr := flag.Uint8P("parallelism", "p", defaultParallelismValue,
		"extent of parallelism (defaults to number of cores minus 1)")
	flags.getParallelism = func() int {
		if *parallelismPtr == defaultParallelismValue {
			n := runtime.NumCPU()
			if n > 1 {
				return n - 1
			}
			return 1
		}
		return int(*parallelismPtr)
	}
}

func setupOutputModeOpt() {
	var sb strings.Builder
	sb.WriteString("following modes are accepted:\n")
	for outputMode, description := range entity.OutputModes {
		sb.WriteString(fmt.Sprintf("%5s = %s\n", outputMode, description))
	}
	outputModeStrPtr := flag.StringP("output", "o", entity.OutputModeTextFile, sb.String())
	flags.getOutputMode = func() string {
		outputModeStr := strings.ToLower(strings.TrimSpace(*outputModeStrPtr))
		if _, exists := entity.OutputModes[outputModeStr]; !exists {
			fmt.Printf("error: invalid output mode '%s'\n", outputModeStr)
			os.Exit(exitCodeInvalidOutputMode)
		}
		return outputModeStr
	}
}

func setupUsage() {
	flag.Usage = func() {
		fmte.PrintfErr("Run \"go-find-duplicates --help\" for usage\n")
	}
}

func readDirectories() (directories []string) {
	if flag.NArg() < 1 {
		fmte.PrintfErr("error: no input directories passed\n")
		flag.Usage()
		os.Exit(exitCodeInvalidNumArgs)
	}
	for i, p := range flag.Args() {
		if !utils.IsReadableDirectory(p) {
			fmte.PrintfErr("error: input #%d \"%v\" isn't a readable directory\n", i+1, p)
			flag.Usage()
			os.Exit(exitCodeInputDirectoryNotReadable)
		}
		abs, _ := filepath.Abs(p)
		directories = append(directories, abs)
	}
	return directories
}

func handlePanic() {
	err := recover()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Program exited unexpectedly. "+
			"Please report the below eror to the author:\n"+
			"%+v\n", err)
		_, _ = fmt.Fprintln(os.Stderr, string(debug.Stack()))
	}
}

func showHelpAndExit() {
	flag.CommandLine.SetOutput(os.Stdout)
	fmt.Printf(`go-find-duplicates is a tool to find duplicate files and directories

Usage:
  go-find-duplicates [flags] <dir-1> <dir-2> ... <dir-n>

where,
  arguments are readable directories that need to be scanned for duplicates

Flags (all optional):
`)
	flag.PrintDefaults()
	fmt.Printf(`
For more details: https://github.com/m-manu/go-find-duplicates
`)
	os.Exit(exitCodeSuccess)
}

func setupFlags() {
	setupExclusionsOpt()
	setupHelpOpt()
	setupMinSizeOpt()
	setupOutputModeOpt()
	setupParallelismOpt()
	setupThoroughOpt()
	setupUsage()
	setupHashSizeOpt()
}

func main() {
	defer handlePanic()
	runID := time.Now().Format("150405")
	setupFlags()
	flag.Parse()
	if flags.isHelp() {
		showHelpAndExit()
	}
	directories := readDirectories()
	outputMode := flags.getOutputMode()
	reportFileName := createReportFileIfApplicable(runID, outputMode)
	duplicates, duplicateTotalCount, savingsSize, allFiles, fdErr :=
		service.FindDuplicates(directories, flags.getExcludedFiles(), flags.getMinSize(),
			flags.getParallelism(), flags.isThorough(), flags.getHashSize())
	if fdErr != nil {
		fmte.PrintfErr("error while finding duplicates: %+v\n", fdErr)
		os.Exit(exitCodeErrorFindingDuplicates)
	}
	if duplicates == nil || duplicates.Size() == 0 {
		if len(allFiles) == 0 {
			fmte.Printf("No actions performed!\n")
		} else {
			fmte.Printf("No duplicates found!\n")
		}
		return
	}
	duplicates = service.RecheckDuplicates(duplicates, flags.getParallelism())
	fmte.Printf("Found %d duplicates. A total of %s can be saved by removing them.\n",
		duplicateTotalCount, bytesutil.BinaryFormat(savingsSize))

	err := reportDuplicates(duplicates, outputMode, allFiles, runID, reportFileName)
	if err != nil {
		fmte.PrintfErr("error while reporting to file: %+v\n", err)
		os.Exit(exitCodeWritingToReportFileFailed)
	}
}

func createReportFileIfApplicable(runID string, outputMode string) (reportFileName string) {
	if outputMode == entity.OutputModeStdOut {
		return
	}
	if outputMode == entity.OutputModeCsvFile {
		reportFileName = fmt.Sprintf("./duplicates_%s.csv", runID)
	} else if outputMode == entity.OutputModeTextFile {
		reportFileName = fmt.Sprintf("./duplicates_%s.txt", runID)
	} else if outputMode == entity.OutputModeJSON {
		reportFileName = fmt.Sprintf("./duplicates_%s.json", runID)
	}
	_, err := os.Create(reportFileName)
	if err != nil {
		fmte.PrintfErr("error: couldn't create report file: %+v\n", err)
		os.Exit(exitCodeReportFileCreationFailed)
	}
	return
}
