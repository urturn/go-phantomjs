package phantomjs

import (
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStartStop(t *testing.T) {
	p, err := Start()
	failOnError(err, t)
	err = p.Exit()
	failOnError(err, t)
}

func TestStartStopWithArgs(t *testing.T) {
	p, err := Start("--web-security=no")
	failOnError(err, t)
	err = p.Exit()
	failOnError(err, t)
}

func TestRunACommand(t *testing.T) {
	p, err := Start()
	defer p.Exit()
	failOnError(err, t)
	assertFloatResult("function(){ return 2 + 1; }\n", 3, p, t)
}

func TestRunACommandWithoutLineBreak(t *testing.T) {
	p, err := Start()
	defer p.Exit()
	failOnError(err, t)
	assertFloatResult("function(){ return 2 + 2; }", 4, p, t)
}

func TestRunAnAsyncCommand(t *testing.T) {
	p, err := Start()
	failOnError(err, t)
	defer p.Exit()
	assertFloatResult("function(done){ done(2 + 3) ; }\n", 5, p, t)
	p1, err := Start()
	failOnError(err, t)
	defer p1.Exit()
	assertFloatResult("function(done){ setTimeout(function() { done(3 + 3) ; }, 0); }\n", 6, p1, t)
}

func TestRunMultilineCommand(t *testing.T) {
	p, err := Start()
	failOnError(err, t)
	defer p.Exit()
	assertFloatResult("function() {\n\t    return 3+4;\n}\n", 7, p, t)
}

func TestRunMultipleCommands(t *testing.T) {
	p, err := Start()
	failOnError(err, t)
	defer p.Exit()
	assertFloatResult("function() {return 1}", 1, p, t)
	assertFloatResult("function() {return 1}", 1, p, t)
	assertFloatResult("function() {return 1}", 1, p, t)
}

func TestLoadGlobal(t *testing.T) {
	p, err := Start()
	failOnError(err, t)
	defer p.Exit()
	p.Load("function result(result) { return result; }\nvar a = 2")
	assertFloatResult("function() {return result(a);}", 2, p, t)
}

func TestMessageSentAfterAnErrorDontCrash(t *testing.T) {
	p, err := Start()
	failOnError(err, t)
	defer p.Exit()
	p.Run("function(done) {done(null, 'manual'); done('should not panic');}", nil)
}

func TestDoubleErrorSendDontCrash(t *testing.T) {
	p, err := Start()
	failOnError(err, t)
	defer p.Exit()
	p.Run("function(done) {done(null, 'manual'); done(null, 'should not panic');}", nil)
}

func TestThrow(t *testing.T) {
	p, err := Start()
	if err != nil {
		panic(err)
	}
	defer p.Exit() // Don't forget to kill phantomjs at some point.
	var result interface{}
	err = p.Run("function() { throw 'Ooops' }", &result)
	if err == nil {
		t.Fatal("Expected an Error")
	}
	if !strings.Contains("\"Ooops\"", err.Error()) {
		t.Fatal(err)
	}
}

func TestComplex(t *testing.T) {
	var wg sync.WaitGroup

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, err := Start()
			failOnError(err, t)
			defer p.Exit()
			var r interface{}
			begin := time.Now()

			p.Run(`function(done){
							var a = 0;
							var b = 1;
							var c = 0;
							for(var i=2; i<=25; i++)
							{
    						c = b + a;
								a = b;
								b = c;
							}
							done(c, undefined);
				}`, &r)
			t.Logf("Completed Run in %s", time.Since(begin))
			failOnError(err, t)
			v, ok := r.(float64)
			if !ok {
				t.Errorf("Should be an int but is %v", r)
				return
			}
			if v != 75025 {
				t.Errorf("Should be %d but is %f", 75025, v)
			}
		}()
	}
	wg.Wait()
}

func TestForceShutdown(t *testing.T) {
	p, err := Start()
	failOnError(err, t)

	count := 0
	for i := 0; i < 5; i++ {
		var r interface{}

		if i == 2 {
			err = p.ForceShutdown()
			failOnError(err, t)
		}
		err = p.Run(`function(done){
							var a = 0;
							var b = 1;
							var c = 0;
							for(var i=2; i<=25; i++)
							{
								c = b + a;
								a = b;
								b = c;
							}
							done(c, undefined);
				}`, &r)
		if err == nil {
			v, ok := r.(float64)
			if !ok {
				t.Errorf("Should be an int but is %v", r)
				return
			}
			if v != 75025 {
				t.Errorf("Should be %d but is %f", 75025, v)
			} else {
				count++
			}
		} else {
			if !strings.Contains(err.Error(), "PhantomJS Instance is dead") && !strings.Contains(err.Error(), "phantomjs instance might be dead") {
				t.Fatal(err)
			}
		}
	}
	if count != 2 {
		t.Fatalf("Didn't reach destination %d", count)
	}
	err = p.Exit()
	failOnError(err, t)
}

func TestRestartInstance(t *testing.T) {
	p, err := Start()
	defer p.Exit()
	failOnError(err, t)

	err = p.ForceShutdown()
	failOnError(err, t)

	p, err = Start()
	defer p.Exit()
	failOnError(err, t)

	var r interface{}

	err = p.Run(`function(done){
						var a = 0;
						var b = 1;
						var c = 0;
						for(var i=2; i<=25; i++)
						{
							c = b + a;
							a = b;
							b = c;
						}
						done(c, undefined);
			}`, &r)

	failOnError(err, t)
	v, ok := r.(float64)
	if !ok {
		t.Errorf("Should be an int but is %v", r)
		return
	}
	if v != 75025 {
		t.Errorf("Should be %d but is %f", 75025, v)
	}
}

func TestMutlipleThreads(t *testing.T) {

	var wg sync.WaitGroup
	var stats runtime.MemStats
	for i := 0; i < 3; i++ {
		wg.Add(1)

		runtime.ReadMemStats(&stats)

		t.Logf("MEM: %d, MALC:  %d, GOTHREAD: %d, %s", (stats.Sys / 1000000), stats.Mallocs, runtime.NumGoroutine(), "Starting GO ROUTINE")
		go func() {
			var insideStats runtime.MemStats
			runtime.ReadMemStats(&insideStats)
			t.Logf("MEM: %d, MALC:  %d, GOTHREAD: %d, %s", (insideStats.Sys / 1000000), insideStats.Mallocs, runtime.NumGoroutine(), "Start of Processor")
			// Staring new Thread
			p, err := Start()
			t.Logf("MEM: %d, MALC:  %d, GOTHREAD: %d, %s", (insideStats.Sys / 1000000), insideStats.Mallocs, runtime.NumGoroutine(), "Started Processor")

			failOnError(err, t)
			var r interface{}
			for j := 0; j < 5; j++ {
				runtime.ReadMemStats(&insideStats)
				t.Logf("MEM: %d, MALC:  %d, GOTHREAD: %d, %s", (insideStats.Sys / 1000000), insideStats.Mallocs, runtime.NumGoroutine(), "Starting RUN && BEGIN OF LOOP")
				err = p.Run(`function(done){
							var a = 0;
							var b = 1;
							var c = 0;
							var page = new WebPage();
							page.open('http://charles.lescampeurs.org/');
							page.onLoadFinished = function(status) {
								for(var i=2; i<=25; i++)
								{
									c = b + a;
									a = b;
									b = c;
								}
								done(c, undefined);
							}
				}`, &r)
				t.Logf("MEM: %d, MALC:  %d, GOTHREAD: %d, %s", (insideStats.Sys / 1000000), insideStats.Mallocs, runtime.NumGoroutine(), "Ending RUN")

				if err == nil {
					v, ok := r.(float64)
					if !ok {
						t.Errorf("Should be an int but is %v", r)
						p.Exit()
						defer wg.Done()
						return
					}
					if v != 75025 {
						t.Errorf("Should be %d but is %f", 75025, v)
					}
				} else {
					if !strings.Contains(err.Error(), "PhantomJS Instance is dead") && !strings.Contains(err.Error(), "phantomjs instance might be dead") {
						t.Fatal(err)
					}
				}
				runtime.ReadMemStats(&insideStats)

				t.Logf("MEM: %d, MALC:  %d, GOTHREAD: %d, %s", (insideStats.Sys / 1000000), insideStats.Mallocs, runtime.NumGoroutine(), "Ending Loop \n ")

			}
			p.Exit()
			defer wg.Done()
			t.Logf("MEM: %d, MALC:  %d, GOTHREAD: %d, %s", (insideStats.Sys / 1000000), insideStats.Mallocs, runtime.NumGoroutine(), "Done with Processor")
			//Thread Done
		}()
		runtime.ReadMemStats(&stats)
		t.Logf("MEM: %d, MALC:  %d, GOTHREAD: %d, %s", (stats.Sys / 1000000), stats.Mallocs, runtime.NumGoroutine(), "Started GO ROUTINE")
	}
	wg.Wait()
	runtime.ReadMemStats(&stats)
	t.Logf("MEM: %d, MALC:  %d, GOTHREAD: %d, %s", (stats.Sys / 1000000), stats.Mallocs, runtime.NumGoroutine(), "Done")
}

func TestMultipleLogs(t *testing.T) {
	p, err := Start()
	defer p.Exit()
	failOnError(err, t)
	var r interface{}

	err = p.Run(`function(done){
						var a = 0;
						var b = 1;
						var c = 0;
						for(var i=2; i<=25; i++)
						{
							c = b + a;
							a = b;
							b = c;
							console.log(c)
						}
						done(c, undefined);
			}`, &r)
	failOnError(err, t)
	v, ok := r.(float64)
	if !ok {
		t.Errorf("Should be an int but is %v", r)
		return
	}
	if v != 75025 {
		t.Errorf("Should be %d but is %f", 75025, v)
	}
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
