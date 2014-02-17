var global = this;
(function() {
  var system = require('system');
  var id = 0;

  /**
   * This method read stdin and extract command from it.
   *
   * There is two kind of commands:
   * EVAL token: evaluate all the upcoming lines up to the END token.
   * RUN token: run the function contained in the upcoming lines, up to the END token.
   *
   * Sample commands
   * ---------------
   * This command exits from PhantomJS
   * > EVAL
   * > phantom.exit();
   * > END
   *
   * This command returns 2
   * > RUN
   * > function() {
   * >    return 2
   * > };
   * > END
   *
   * This command return the status code of a page load
   * > RUN
   * > function(done) {
   * >   var page = require('page')
   * >   page.open('http://www.example.com', function(status) {
   * >     done(status);
   * >   });
   * > }
   * > END
   */
  function captureInput() {
    var lines = [];
    system.stdout.writeLine('[WAITING]');
    var l = system.stdin.readLine();
    while (l !== 'END') {
      lines.push(l);
      l = system.stdin.readLine();
    }
    var command = lines.splice(0, 1)[0];
    if (command === 'EVAL') {
      try {
        eval.call(this, lines.join('\n'));
      } catch (ex) {
        system.stdout.writeLine("Error during EVAL of" + lines.join('\n'));
      }
      setTimeout(captureInput, 0);
    } else if (command === 'RUN') {
      evaluate(id++, lines.join('\n'));
    } else {
      system.stdout.writeLine("Invalid command:<" + command+">");
      setTimeout(captureInput, 0);
    }
  }

  /**
   * Evaluate the given input, a string representing a function declaration.
   *
   * The result or error of the function will be given to a function produced
   * by doneFunc(id) below.
   *
   * if the function has a parameter, this parameter will be set to the doneFunc(id)
   * member. If not, the returned value will be passed to doneFunc(id) member.
   *
   * @param  {string} id    unique identifier for the current run
   * @param  {string} input source code of a function
   */
  function evaluate(id, input) {
    var func, args;
    system.stdout.writeLine("NEW" + id + " " + input);
    try {
      eval("func = " + input);
      args = func && (args = func.toString().match(/^function\s+(?:\w+)?\(([^\)]*)\)/m)[1].replace(/\s+/, "")) && args.split(',');
      if (args) {
        func(doneFunc("RES"+id));
      } else {
        try {
          doneFunc("RES"+id)(func());
        } catch (ex) {
          doneFunc("RES"+id)(null, ex);
        }
      }
    } catch (ex) {
      setTimeout(captureInput, 0);
    }
  }

  /**
   * Retrieve a wrapper function that can be passed a result and an error.
   *
   * @param  {string} id identify the current command
   * @return {function}  a function that accept a result and an error parameter
   */
  function doneFunc(id) {
    return function (result, err) {
      if (err) {
        system.stderr.writeLine(id + " " + JSON.stringify(err) + "\n");
      } else {
        system.stdout.writeLine(id + " " + JSON.stringify(result) + "\n");
      }
      setTimeout(captureInput, 0);
    };
  }

  setTimeout(captureInput, 0);
}());