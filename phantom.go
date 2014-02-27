package phantomjs

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
)

type Phantom struct {
	cmd    *exec.Cmd
	in     io.WriteCloser
	out    io.ReadCloser
	errout io.ReadCloser
}

var nbInstance = 0
var wrapperFileName = ""

/*
Create a new `Phantomjs` instance and return it as a pointer.

If an error occurs during command start, return it instead.
*/
func Start(args ...string) (*Phantom, error) {
	if nbInstance == 0 {
		wrapperFileName, _ = createWrapperFile()
	}
	nbInstance += 1
	args = append(args, wrapperFileName)
	cmd := exec.Command("phantomjs", args...)

	inPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	outPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	errPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	p := Phantom{
		cmd:    cmd,
		in:     inPipe,
		out:    outPipe,
		errout: errPipe,
	}
	err = cmd.Start()

	if err != nil {
		return nil, err
	}

	return &p, nil
}

/*
Exit Phantomjs by sending the "phantomjs.exit()" command
and wait for the command to end.

Return an error if one occured during exit command or if the program output a error value
*/
func (p *Phantom) Exit() error {
	err := p.Load("phantom.exit()")
	if err != nil {
		return err
	}

	err = p.cmd.Wait()
	if err != nil {
		return err
	}
	nbInstance -= 1
	if nbInstance == 0 {
		os.Remove(wrapperFileName)
	}

	return nil
}

/*
Run the javascript function passed as a string and wait for the result.

The result can be either in the return value of the function or the first argument passed
to the function first arguments.
*/
func (p *Phantom) Run(jsFunc string, res *interface{}) error {
	err := p.sendLine("RUN", jsFunc, "END")
	if err != nil {
		return err
	}
	scannerOut := bufio.NewScanner(p.out)
	scannerErrorOut := bufio.NewScanner(p.errout)
	resMsg := make(chan string)
	errMsg := make(chan error)
	go func() {
		for scannerOut.Scan() {
			line := scannerOut.Text()
			parts := strings.SplitN(line, " ", 2)
			if strings.HasPrefix(line, "RES") {
				resMsg <- parts[1]
				close(resMsg)
				return
			} else {
				fmt.Printf("LOG %s\n", line)
			}
		}
	}()
	go func() {
		for scannerErrorOut.Scan() {
			line := scannerErrorOut.Text()
			parts := strings.SplitN(line, " ", 2)
			if strings.HasPrefix(line, "RES") {
				errMsg <- errors.New(parts[1])
				close(errMsg)
				return
			} else {
				fmt.Printf("LOG %s\n", line)
			}
		}
	}()
	select {
	case text := <-resMsg:
		if res != nil {
			err = json.Unmarshal([]byte(text), res)
			if err != nil {
				return err
			}
		}
		return nil
	case err := <-errMsg:
		return err
	}
}

/*
Eval `jsCode` in the main context.
*/
func (p *Phantom) Load(jsCode string) error {
	return p.sendLine("EVAL", jsCode, "END")
}

func createWrapperFile() (fileName string, err error) {
	wrapper, err := ioutil.TempFile("", "go-phantom-wrapper")
	if err != nil {
		return "", err
	}
	defer wrapper.Close()

	wrapperData, err := Asset("data/wrapper.js")
	if err != nil {
		return "", err
	}

	err = ioutil.WriteFile(wrapper.Name(), wrapperData, os.ModeType)
	if err != nil {
		return "", err
	}
	return wrapper.Name(), nil
}

func (p *Phantom) sendLine(lines ...string) error {
	for _, l := range lines {
		_, err := io.WriteString(p.in, l+"\n")
		if err != nil {
			return errors.New("Cannot Send: `" + l + "`")
		}
	}
	return nil
}
