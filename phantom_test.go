package phantomjs

import (
	"testing"
)

func TestStartStop(t *testing.T) {
	p, err := Start("phantomjs")
	failOnError(err, t)
	err = p.Exit()
	failOnError(err, t)
}

func TestStartStopWithArgs(t *testing.T) {
	p, err := Start("phantomjs", "--web-security=no")
	failOnError(err, t)
	err = p.Exit()
	failOnError(err, t)
}

func TestRunACommand(t *testing.T) {
	p, err := Start("phantomjs")
	defer p.Exit()
	failOnError(err, t)
	assertFloatResult("function(){ return 2 + 1; }\n", 3, p, t)
}

func TestRunACommandWithoutLineBreak(t *testing.T) {
	p, err := Start("phantomjs")
	defer p.Exit()
	failOnError(err, t)
	assertFloatResult("function(){ return 2 + 2; }", 4, p, t)
}

func TestRunAnAsyncCommand(t *testing.T) {
	p, err := Start("phantomjs")
	failOnError(err, t)
	defer p.Exit()
	assertFloatResult("function(done){ done(2 + 3) ; }\n", 5, p, t)
	p1, err := Start("phantomjs")
	failOnError(err, t)
	defer p1.Exit()
	assertFloatResult("function(done){ setTimeout(function() { done(3 + 3) ; }, 0); }\n", 6, p1, t)
}

func TestRunMultilineCommand(t *testing.T) {
	p, err := Start("phantomjs")
	failOnError(err, t)
	defer p.Exit()
	assertFloatResult("function() {\n\t    return 3+4;\n}\n", 7, p, t)
}

func TestRunMultipleCommands(t *testing.T) {
	p, err := Start("phantomjs")
	failOnError(err, t)
	defer p.Exit()
	assertFloatResult("function() {return 1}", 1, p, t)
	assertFloatResult("function() {return 1}", 1, p, t)
	assertFloatResult("function() {return 1}", 1, p, t)
}

func TestLoadGlobal(t *testing.T) {
	p, err := Start("phantomjs")
	failOnError(err, t)
	defer p.Exit()
	p.Load("function result(result) { return result; }\nvar a = 2")
	assertFloatResult("function() {return result(a);}", 2, p, t)
}

func TestMessageSentAfterAnErrorDontCrash(t *testing.T) {
	p, err := Start("phantomjs")
	failOnError(err, t)
	defer p.Exit()
	p.Run("function(done) {done(null, 'manual'); done('should not panic');}", nil)
}

func TestDoubleErrorSendDontCrash(t *testing.T) {
	p, err := Start("phantomjs")
	failOnError(err, t)
	defer p.Exit()
	p.Run("function(done) {done(null, 'manual'); done(null, 'should not panic');}", nil)
}

func assertFloatResult(jsFunc string, expected float64, p *Phantom, t *testing.T) {
	var r interface{}
	err := p.Run(jsFunc, &r)
	failOnError(err, t)
	v, ok := r.(float64)
	if !ok {
		t.Errorf("Should be an int but is %v", r)
		return
	}
	if v != expected {
		t.Errorf("Should be %f but is %f", expected, v)
	}
}

func failOnError(err error, t *testing.T) {
	if err != nil {
		t.Fatal(err)
	}
}
