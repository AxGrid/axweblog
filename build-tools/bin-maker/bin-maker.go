package main

import (
	"flag"
	"fmt"
	"github.com/axgrid/axweblog/shared/ospathlib"
	"github.com/logrusorgru/aurora"
	"github.com/olekukonko/tablewriter"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var root string

const tag = "-build-me-for:"

type BuildPath struct {
	path         string
	cwd          string
	instructions []string
}

var kids = 0

func init() {
	flag.StringVar(&root, "root", "", "go binaries search root")
}

func main() {

	flag.Parse()
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).Level(zerolog.InfoLevel)
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	var result []*BuildPath

	err = filepath.Walk(filepath.Join(cwd, root), func(path string, info fs.FileInfo, err error) error {
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".go") {
			return nil
		}
		relativePath, err := ospathlib.SubtractPath(path, cwd+"/")
		if err != nil {
			log.Fatal().Err(err).Msg("relative path")
		}
		if relativePath == "" {
			relativePath = "."
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".go") {
			res, err := processFile(relativePath, cwd)
			if err != nil {
				return err
			}
			if res == nil || len(res) == 0 {
				return nil
			}
			result = append(result, &BuildPath{
				path:         relativePath,
				cwd:          cwd,
				instructions: res,
			})
		}
		return nil
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Panic")
	}

	errorChan := make(chan *BuildError)

	for _, r := range result {
		launchTheBuilds(r, errorChan, &kids)
	}

	tw := tablewriter.NewWriter(os.Stdout)
	tw.SetHeader([]string{
		"bin",
		"target",
		"cmd",
		"duration",
	})
	tw.SetRowLine(true)
	tw.SetRowSeparator("-")

	var okTableContent [][]string
	for i := 0; i < kids; i++ {
		done := <-errorChan
		log.Info().Interface("d", done).Msg("done")
		if done.SErr != nil && len(done.SErr) > 0 {
			log.Fatal().Str("file", done.File).Str("arch", done.Arch).Msg(string(done.SErr))
		}
		okTableContent = append(okTableContent, []string{
			done.File,
			done.Arch,
			done.Command,
			fmt.Sprintf("%v", aurora.Green(done.Duration)),
		})
		//log.Info().Interface("done", done).Msg("finish")
	}
	tw.AppendBulk(okTableContent)
	tw.Render()
}

func processFile(relativePath string, cwd string) ([]string, error) {
	log.Debug().Str("relative", relativePath).Str("cwd", cwd).Msgf("Process")
	return getBuildComment(relativePath), nil
}

func getBuildComment(path string) []string {
	astFile, err := tryParseGoFile(path)
	if err != nil {
		log.Fatal().Err(err).Msg("parseFileError")
	}
	if hasMainFunction(astFile) {
		instr := parseBuildInstructions(astFile)
		if len(instr) > 0 {
			return instr
		}
	}
	return nil
}

func parseBuildInstructions(f *ast.File) []string {
	var targets []string
	for _, comment := range f.Comments {
		parts := strings.Split(comment.Text(), "\n")
		for _, c := range parts {
			if strings.HasPrefix(c, tag) {
				tokens := strings.Split(c, ":")
				if len(tokens) == 2 {
					platforms := strings.Split(tokens[1], ",")
					for _, platform := range platforms {
						platform = sanityFix(platform)
						targets = append(targets, platform)
					}
				}
			}
		}
	}

	return targets
}

func hasMainFunction(f *ast.File) bool {
	if f.Name.Name == "main" {
		for _, d := range f.Decls {
			if gen, ok := d.(*ast.FuncDecl); ok {
				if gen.Name.Name == "main" {
					return true
				}
			}
		}
	}

	return false
}

func launchTheBuilds(bp *BuildPath, doneChan chan *BuildError, kids *int) {

	pathTokens := strings.Split(bp.path, "/")
	fileName := pathTokens[len(pathTokens)-1]
	binaryName := strings.ReplaceAll(fileName, ".go", "")
	//ts := time.Now()
	for _, srcString := range bp.instructions {
		target := getGoBuildTarget(srcString)
		tags := getGoBuildTags(srcString)
		goEnv, err := getGoBuildPrefix(target)
		extras := ""
		if err != nil {
			log.Fatal().Msgf("error processing '%s' directive... for %v!\n", tag, bp.path)
		}
		var outPath string
		var goBin = "go"
		var goMods = "-mod vendor"
		var goTags = ""
		if len(tags) > 0 {
			goTags = "-tags \""
			goTags += strings.Join(tags, " ")
			goTags += "\""
		}

		if len(tags) > 0 {
			outPath = fmt.Sprintf("target/%s-%s.%s", binaryName, strings.Join(tags, "-"), target)
		} else {
			cleanedTarget := target
			if strings.HasSuffix(cleanedTarget, "-docker") {
				cleanedTarget = strings.TrimSuffix(cleanedTarget, "-docker")
			}
			outPath = fmt.Sprintf("target/%s.%s", binaryName, cleanedTarget)
		}

		cmd := ""
		if goEnv == "" {
			cmd = fmt.Sprintf("%s build %s -o %s %s", goBin, goMods, outPath, bp.path)
		} else {
			cmd = fmt.Sprintf("%s %s build %s %s %s -o %s %s", goEnv, goBin, goMods, goTags, extras, outPath, bp.path)
		}
		//log.Printf("RUN [%s]\n", cmd)
		//log.Info().Msgf("cmd:%s", cmd)
		*kids++
		go runTimedCommand(fileName, srcString, cmd, doneChan)
	}

}

func getGoBuildTarget(src string) string {
	return strings.Split(src, "+")[0]
}

func getGoBuildTags(src string) []string {
	return strings.Split(src, "+")[1:]
}

func runTimedCommand(file string, arch string, cmd string, doneChannel chan *BuildError) {
	ts := time.Now()
	process := exec.Command("/bin/sh", "-c", cmd)
	process.Stdin = os.Stdin
	stdout, err := process.StdoutPipe()
	if err != nil {
		doneChannel <- mkBuildError(file, arch, cmd, err, nil, nil, time.Since(ts))
		return
	}
	stderr, err := process.StderrPipe()
	if err != nil {
		doneChannel <- mkBuildError(file, arch, cmd, err, nil, nil, time.Since(ts))
		return
	}

	err = process.Start()
	if err != nil {
		doneChannel <- mkBuildError(file, arch, cmd, err, nil, nil, time.Since(ts))
		return
	}

	so, _ := ioutil.ReadAll(stdout)
	se, _ := ioutil.ReadAll(stderr)

	if len(so) > 0 {
		// log.Println(string(so))
	}
	if len(se) > 0 {
		// log.Println(string(se))
	}

	if err = process.Wait(); err != nil {
		doneChannel <- mkBuildError(file, arch, cmd, err, so, se, time.Since(ts))
		return
	}

	// log.Printf("DONE (in %v) [%s]\n", time.Since(ts), cmd)
	doneChannel <- mkBuildError(file, arch, cmd, err, so, se, time.Since(ts))
}

func getGoBuildPrefix(target string) (string, error) {
	switch target {
	case "native":
		return "", nil
	case "arm":
		return "GOOS=linux GOARCH=arm64", nil
	case "arm-docker":
		return "GOOS=linux GOARCH=arm64", nil
	case "windows":
		return "GOOS=windows GOARCH=amd64", nil
	case "linux":
		return "GOOS=linux GOARCH=amd64", nil
	case "osx":
		return "GOOS=darwin GOARCH=amd64", nil
	case "wasm":
		return "GOOS=js GOARCH=wasm", nil
	}

	return "", fmt.Errorf("unknown binary target in %s tag", target)
}

var whitespace = []string{" ", ",", "\n", "\t"}

func sanityFix(src string) string {
	for _, ws := range whitespace {
		src = strings.ReplaceAll(src, ws, "")
	}

	return src
}

func tryParseGoFile(path string) (*ast.File, error) {
	fset := token.NewFileSet()
	src, err := ioutil.ReadFile(path)
	if err != nil {
		log.Printf("error reading file %s, with error %v\n", path, err)
		return nil, err
	}
	return parser.ParseFile(fset, path, src, parser.ParseComments)
}

type BuildError struct {
	Err      error
	Command  string
	SOut     []byte
	SErr     []byte
	Duration time.Duration
	File     string
	Arch     string
}

func mkBuildError(file string, arch string, cmd string, err error, sout []byte, serr []byte, duration time.Duration) *BuildError {
	return &BuildError{
		Err:      err,
		Command:  cmd,
		SOut:     sout,
		SErr:     serr,
		Duration: duration,
		File:     file,
		Arch:     arch,
	}
}
