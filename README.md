go-phantomjs
============

A tiny phantomjs wrapper for go

Usage
```
import (
  "github.com/urturn/go-phantomjs" // exported package is phantomjs
)

func main() {
  p, err := phantomjs.Start()
  if err != nil {
    panic(err)
  }
  var result interface{}
  err = p.Run("function() { return 2 + 2 }", &result)
  if err != nil {
    panic(err)
  }
  number, ok := result.(float64)
  if !ok {
    panic("Cannot convert result to float64")
  }
  fmt.Println(number)
  // Output: 4
}
```
