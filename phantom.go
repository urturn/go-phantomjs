package phantomjs

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// Phantom a data structure that interacts with the wrapper file
type Phantom struct {
	cmd              *exec.Cmd
	in               io.WriteCloser
	out              io.ReadCloser
	errout           io.ReadCloser
	scannerErrorLock *sync.Mutex
	scannerLock      *sync.Mutex
}

var nbInstance = 0

var wrapperFileName = ""
var fileLock = new(sync.Mutex)

/*
Start create phantomjs file
Create a new `Phantomjs` instance and return it as a pointer.

If an error occurs during command start, return it instead.
*/
func Start(args ...string) (*Phantom, error) {
	fileLock.Lock()
	if nbInstance == 0 {
		wrapperFileName, _ = createWrapperFile()
	}
	nbInstance++
	fileLock.Unlock()
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
		cmd:              cmd,
		in:               inPipe,
		out:              outPipe,
		errout:           errPipe,
		scannerErrorLock: new(sync.Mutex),
		scannerLock:      new(sync.Mutex),
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
	p.scannerErrorLock.Lock()
	p.scannerLock.Lock()
	err = p.cmd.Wait()
	p.scannerErrorLock.Unlock()
	p.scannerLock.Unlock()
	if err != nil {
		return err
	}
	fileLock.Lock()
	nbInstance--
	if nbInstance == 0 {
		os.Remove(wrapperFileName)
	}
	fileLock.Unlock()
	return nil
}

/*
ForceShutdown will forcefully kill phantomjs.
This will completly terminate the proccess compared to Exit which will safely Exit
*/
func (p *Phantom) ForceShutdown() error {
	if err := p.cmd.Process.Kill(); err != nil {
		return err
	}
	fileLock.Lock()
	nbInstance--
	if nbInstance == 0 {
		os.Remove(wrapperFileName)
	}
	fileLock.Unlock()
	return nil
}

/*
Run the javascript function passed as a string and wait for the result.

The result can be either in the return value of the function or,
You can pass a function a closure which can take two arguments 1st is the successfull response
the 2nd is an err. See TestComplex in the phantom_test.go
*/
func (p *Phantom) Run(jsFunc string, res *interface{}) error {
	err := p.sendLine("RUN", jsFunc, "END")
	if err != nil {
		return err
	}
	scannerOut := NewPhantomScanner(p.out)
	scannerErrorOut := NewPhantomScanner(p.errout)
	resMsg := make(chan string)
	errMsg := make(chan error)
	go func() {
		value, scanError := readScanner(p.scannerLock, scannerOut)

		if scanError != nil {
			errMsg <- scanError
			return
		}

		if value == "" {
			// Nothing to see here
			return
		}

		resMsg <- value
	}()
	go func() {
		value, scanError := readScanner(p.scannerErrorLock, scannerErrorOut)
		if scanError != nil {
			errMsg <- scanError
			return
		}

		if value == "" {
			// Nothing to see here
			return
		}

		errMsg <- errors.New(value)
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
Load will load more code
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

func readScanner(scannerLock *sync.Mutex, scanner *PhantomScanner) (string, error) {
	read := true
	for read {
		scannerLock.Lock()
		read := scanner.Scan()
		scannerLock.Unlock()
		if !read {
			return "", errors.New("phantomjs instance is no longer running")
		}

		if scanner.Err() != nil {
			return "", scanner.Err()
		}

		line := scanner.Text()
		parts := strings.SplitN(line, " ", 2)

		if strings.HasPrefix(line, "RES") {
			return parts[1], nil
		} else if line != "" {
			fmt.Printf("LOG %s\n", line)
			// return "", nil
		} else if line != " " {
			// return "", errors.New("Error reading response, just got a space")
		}
	}
	return "", errors.New("No Response")
}

func (p *Phantom) sendLine(lines ...string) error {
	for _, l := range lines {
		_, err := io.WriteString(p.in, l+"\n")
		if err != nil {
			return errors.New("Cannot Send: `" + l + "`" + "phantomjs instance might be dead")
		}
	}
	return nil
}
