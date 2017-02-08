package phantomjs

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

// Phantom a data structure that interacts with the wrapper file
type Phantom struct {
	cmd              *exec.Cmd
	in               io.WriteCloser
	out              io.ReadCloser
	errout           io.ReadCloser
	readerErrorLock  *sync.Mutex
	readerLock       *sync.Mutex
	nothingReadCount int64
}

// return Value is used to bundle up the value returned from the reader
type returnValue struct {
	val string
	err error
}

var nbInstance = 0

var wrapperFileName = ""
var fileLock = new(sync.Mutex)

var readerBufferSize = 2048
var maxReadTimes = 100

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
		readerErrorLock:  new(sync.Mutex),
		readerLock:       new(sync.Mutex),
		nothingReadCount: 0,
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
	p.readerErrorLock.Lock()
	p.readerLock.Lock()
	err = p.cmd.Wait()
	p.readerErrorLock.Unlock()
	p.readerLock.Unlock()
	if err != nil {
		log.Println(err)
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

	err := p.cmd.Process.Kill()

	fileLock.Lock()
	nbInstance--
	if nbInstance == 0 {
		os.Remove(wrapperFileName)
	}
	fileLock.Unlock()
	return err
}

/*
SetMaxBufferSize will set the max Buffer Size
If your script will return a large input use this.
Specify the number of KB
Default value is 2048KB
*/
func (p *Phantom) SetMaxBufferSize(bufferSize int) {
	if bufferSize > 0 {
		readerBufferSize = bufferSize
	}
}

/*
SetMaxReadTimes will set the max read times
If the
*/
func (p *Phantom) SetMaxReadTimes(bufferSize int) {
	if bufferSize > 0 {
		readerBufferSize = bufferSize
	}
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

	readerOut := bufio.NewReaderSize(p.out, readerBufferSize*1024)
	readerErrorOut := bufio.NewReaderSize(p.errout, readerBufferSize*1024)
	resMsg := make(chan string, 1)
	errMsg := make(chan error, 1)
	quit := make(chan bool, 1)

	// var wg sync.WaitGroup
	// wg.Add(1)
	go func() {
		select {
		case value := <-readreader(p.readerLock, readerOut):

			if value.err != nil {
				errMsg <- value.err
				return
			}

			if value.val == "" {
				// Nothing to see here
				return
			}

			resMsg <- value.val
		case <-quit:
			return
		}
	}()
	go func() {
		select {
		case value := <-readreader(p.readerErrorLock, readerErrorOut):
			if value.err != nil {
				errMsg <- value.err
				return
			}

			if value.val == "" {
				// Nothing to see here
				return
			}

			errMsg <- errors.New(value.val)
		case <-quit:
			return
		}
	}()

	// Wait for Threads to complete
	select {
	case text := <-resMsg:
		if res != nil {
			err = json.Unmarshal([]byte(text), res)
			if err != nil {
				return err
			}
		}
		log.Printf("ret val res GT %d", runtime.NumGoroutine())

		// wg.Wait()
		return nil
	case err := <-errMsg:
		if strings.Compare(err.Error(), "EOF") == 0 {
			return errors.New("PhantomJS is no longer running")
		}
		log.Printf("ret val err GT %d", runtime.NumGoroutine())

		// wg.Wait()
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

func readreader(readerLock *sync.Mutex, reader *bufio.Reader) chan returnValue {
	var count = 0
	retVal := make(chan returnValue, 1)
	for {
		readerLock.Lock()
		data, _, err := reader.ReadLine()
		readerLock.Unlock()

		if err != nil {
			// Nothing else to read
			if strings.Compare("EOF", err.Error()) == 0 && len(data) == 0 {
				// like the scanner if 100 empty reads panic
				count++
				if count >= 100 {
					break
				}
			}
			// Nothing to read and waiting to exit
			if len(data) == 0 && strings.Contains(err.Error(), "bad file descriptor") {
				retVal <- returnValue{"", errors.New("Wrapper Error: Bad File Descriptor")}
				return retVal
			} else if strings.Compare("EOF", err.Error()) != 0 && len(data) == 0 {
				retVal <- returnValue{"", err}
				return retVal
			}
		}

		line := string(data)
		parts := strings.SplitN(line, " ", 2)

		if strings.HasPrefix(line, "RES") {
			retVal <- returnValue{parts[1], nil}
			return retVal
		} else if line != "" {
			fmt.Printf("LOG %s\n", line)
			// return "", nil
		} else if line != " " {
			// return "", errors.New("Error reading response, just got a space")
		}
	}
	retVal <- returnValue{"", errors.New("EOF")}
	return retVal
}

func (p *Phantom) sendLine(lines ...string) error {
	for _, l := range lines {
		_, err := io.WriteString(p.in, l+"\n")
		if err != nil {
			return errors.New("Cannot Send: `" + l + "` " + "phantomjs instance might be dead")
		}
	}
	return nil
}
