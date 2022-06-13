package shell_runner

import (
	"io/ioutil"
	"log"
	"os/exec"
	"time"
)

type TimedCommandResult struct {
	Error  error
	StdOut string
	StdErr string
}

func RunTimedCommand(cmd string, doneChannel chan *TimedCommandResult) {
	ts := time.Now()
	process := exec.Command("/bin/sh", "-c", cmd)
	stdout, err := process.StdoutPipe()
	if err != nil {
		doneChannel <- &TimedCommandResult{
			Error: err,
		}
		return
	}
	defer stdout.Close()

	stderr, err := process.StderrPipe()
	if err != nil {
		doneChannel <- &TimedCommandResult{
			Error: err,
		}
		return
	}
	defer stderr.Close()

	err = process.Start()
	if err != nil {
		doneChannel <- &TimedCommandResult{
			Error: err,
		}
		return
	}

	so, _ := ioutil.ReadAll(stdout)
	se, _ := ioutil.ReadAll(stderr)

	if len(so) > 0 {
		log.Println(string(so))
	}
	if len(se) > 0 {
		log.Println(string(se))
	}

	err = process.Wait()

	log.Printf("DONE (in %v) [%s]\n", time.Since(ts), cmd)
	doneChannel <- &TimedCommandResult{
		Error:  err,
		StdOut: string(so),
		StdErr: string(se),
	}
}
