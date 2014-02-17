(function() {
  var system = require('system');
  var id = 0;

  function captureInput() {
    var lines = [];
    system.stdout.writeLine('[WAITING]');
    var l = system.stdin.readLine();
    while (l !== 'END') {
      lines.push(l);
      l = system.stdin.readLine();
    }
    evaluate(id++, lines.join('\n'));
  }

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

  setTimeout(captureInput, 0);
}());