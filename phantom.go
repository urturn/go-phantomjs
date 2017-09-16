package phantomjs

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Phantom a data structure that interacts with the wrapper file
type Phantom struct {
	cmd             *exec.Cmd
	in              io.WriteCloser
	out             io.ReadCloser
	errout          io.ReadCloser
	readerErrorLock *sync.Mutex
	readerLock      *sync.Mutex
	readerOut       *bufio.Reader
	readerErr       *bufio.Reader
	lineOut         chan string
	lineErr         chan string
	quit            chan bool
	stopReading     bool
	once            *sync.Once

	nothingReadCount int64
}

// return Value is used to bundle up the value returned from the reader
type returnValue struct {
	val string
	err error
}

// return Value is used to bundle up the value returned from the reader
type exitValue struct {
	val []byte
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

var cmd = "phantomjs"

// SetCommand lets you specify the binary for phantomjs
func SetCommand(cmd string) {
	cmd = cmd
}

func Start(args ...string) (*Phantom, error) {
	fileLock.Lock()
	if nbInstance == 0 {
		wrapperFileName, _ = createWrapperFile()
	}
	nbInstance++
	fileLock.Unlock()
	args = append(args, wrapperFileName)
	cmd := exec.Command(cmd, args...)

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
		lineOut:          make(chan string, 100),
		lineErr:          make(chan string, 100),
		quit:             make(chan bool, 1),
		stopReading:      false,
		once:             new(sync.Once),
		nothingReadCount: 0,
	}
	p.readerOut = bufio.NewReaderSize(p.out, readerBufferSize*1024)
	p.readerErr = bufio.NewReaderSize(p.errout, readerBufferSize*1024)

	go p.readFromSTDOut()
	go p.readFromSTDERR()

	err = cmd.Start()
	if err != nil {
		return nil, err
	}

	// time.Sleep(time.Millisecond)
	return &p, nil
}

func readreader(readerLock *sync.Mutex, reader *bufio.Reader) (string, error) {
	var count = 0
	for {
		readerLock.Lock()
		data, _, err := reader.ReadLine()
		readerLock.Unlock()

		if err != nil {
			// Nothing else to read
			if err == io.EOF && len(data) == 0 {
				// like the scanner if 100 empty reads panic
				count++
				if count >= 100 {
					break
				}
			}
			// Nothing to read and waiting to exit
			if len(data) == 0 && strings.Contains(err.Error(), "bad file descriptor") {
				return "", errors.New("Wrapper Error: Bad File Descriptor")
			} else if err == io.EOF && len(data) == 0 {
				return "", err
			}
		}

		line := string(data)
		parts := strings.SplitN(line, " ", 2)

		if strings.HasPrefix(line, "RES") {
			return parts[1], nil
		} else if line != "" {
			log.Printf("JS-LOG %s\n", line)
			// return "", nil
		} else if line != " " {
			// return "", errors.New("Error reading response, just got a space")
		}
	}
	return "", errors.New("EOF")
}

func (p *Phantom) readFromSTDOut() {
	for !p.stopReading {
		line, err := readreader(p.readerLock, p.readerOut)
		if err == io.EOF && line == "" {
			// Done Reading Data
			return
		} else if err != nil && !p.stopReading {
			// p.Exit()
			log.Printf("An error occurred while reading stdout: %+v", err)
			return
		} else if p.stopReading && line == "" {
			return
		} else if line == "" {
			continue
		}

		select {
		case p.lineOut <- line:
		default:
			log.Println("no listener attached to stdout " + line)
		}
	}
}

func (p *Phantom) readFromSTDERR() {
	for !p.stopReading {
		line, err := readreader(p.readerErrorLock, p.readerErr)
		if err == io.EOF && line == "" {
			// Done Reading Data
			return
		} else if err != nil && !p.stopReading {
			// p.Exit()
			log.Printf("an error occurred while reading stderr: %+v", err)
			return
		} else if p.stopReading && line == "" {
			return
		} else if line == "" {
			continue
		}

		select {
		case p.lineErr <- line:
		default:
			log.Println("no listener attached to sderr " + line)
		}
	}
}

func drainBool(commch chan bool) {
	for {
		select {
		case <-commch:
		default:
			return
		}
	}
}

/*
Exit Phantomjs by sending the "phantomjs.exit()" command
and wait for the command to end.

Return an error if one occurred during exit command or if the program output a error value
*/
func (p *Phantom) Exit() error {
	var err error
	p.once.Do(func() {
		err = p.Load("phantom.exit()")
		if err != nil {
			return
		}
		p.quit <- true

		p.readerErrorLock.Lock()
		p.readerLock.Lock()
		err = p.cmd.Wait()
		p.readerLock.Unlock()
		p.readerErrorLock.Unlock()

		fileLock.Lock()
		nbInstance--
		if nbInstance == 0 {
			err = os.Remove(wrapperFileName)
		}
		fileLock.Unlock()
	})

	return err
}

/*
ForceShutdown will forcefully kill phantomjs.
This will completly terminate the proccess compared to Exit which will safely Exit
*/
func (p *Phantom) ForceShutdown() error {
	var err error
	p.once.Do(func() {
		err = p.Load("phantom.exit()")
		if err != nil {
			return
		}
		p.quit <- true

		p.readerErrorLock.Lock()
		p.readerLock.Lock()
		err = stopExec(p.cmd)
		p.readerLock.Unlock()
		p.readerErrorLock.Unlock()

		fileLock.Lock()
		nbInstance--
		if nbInstance == 0 {
			err = os.Remove(wrapperFileName)
		}
		fileLock.Unlock()
	})

	if !p.cmd.ProcessState.Exited() {
		err = p.cmd.Process.Kill()
		err = p.cmd.Process.Release()

		fileLock.Lock()
		nbInstance--
		if nbInstance == 0 {
			err = os.Remove(wrapperFileName)
		}
		fileLock.Unlock()
	}

	return err
}

func stopExec(cmd *exec.Cmd) error {
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	select {
	case <-time.After(3 * time.Second):
		if err := cmd.Process.Kill(); err != nil {
			errOther := cmd.Process.Release()
			if errOther != nil {
				log.Printf("Error not Handled %s", errOther)
			}
			return err
		}
		return nil
	case err := <-done:
		if err != nil {
			return err
		}
		return nil
	}
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

func drainchan(commch chan string) {
	for {
		select {
		case <-commch:
		default:
			return
		}
	}
}

/*
Run the javascript function passed as a string and wait for the result.

The result can be either in the return value of the function or,
You can pass a function a closure which can take two arguments 1st is the successfull response
the 2nd is an err. See TestComplex in the phantom_test.go
*/
func (p *Phantom) Run(jsFunc string, res *interface{}) error {
	if p.stopReading {
		return errors.New("PhantomJS Instance is dead")
	}
	// flushing channel incase it read some left over data
	drainchan(p.lineOut)
	drainchan(p.lineErr)
	// end flush
	err := p.sendLine("RUN", jsFunc, "END")
	if err != nil {
		return err
	}
	select {
	case text := <-p.lineOut:
		if res != nil {

			err = json.Unmarshal([]byte(text), res)
			if err != nil {
				return err
			}
		}
		return nil
	case errLine := <-p.lineErr:
		return errors.New(errLine)
	case <-p.quit:
		return errors.New("PhantomJS Instance Killed")
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

func (p *Phantom) sendLine(lines ...string) error {
	for _, l := range lines {
		_, err := io.WriteString(p.in, l+"\n")
		if err != nil {
			return errors.New("Cannot Send: `" + l + "` " + "phantomjs instance might be dead")
		}
	}
	return nil
}
