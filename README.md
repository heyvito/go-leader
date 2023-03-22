# go-leader

> A minimalistic library for leader election

**go-leader** implements a small library for allowing multiple processes to 
elect a single leader and ensure only one of them is the leader for each term.
It leverages Redis to coordinate communication between processes.

## Usage

Configuration is simple. A process must provide a `key`, which must uniquely
identify that workload in the Redis database the provided client is connected
to. This basically means that that key must be the same for all processes 
participating in the election. 

Two more important parameters must be provided: The `TTL` and `Wait` values.
Those are `time.Duration` values indicating how long each lease will last, and
how long processes will wait between elections before starting a new one. It is
important to notice that TTL should be greater than Wait, but it is not required
in case you know what you are doing.

Finally, along with the `Leader` instance, the library will also return three
channels that will receive values whenever the current instance is promoted,
demoted, or reaches an error condition. Those channels MUST be serviced in order
to ensure normal operation. This can be done with a goroutine.

```go
package main

import (
	"fmt"
	
	"github.com/heyvito/go-leader/leader"
)

func main() {
	redisClient := /* Setup Redis client */
	
	procLeader, promoted, demoted, errored := leader.NewLeader(leader.Opts{
	    Redis: redisClient,
		TTL: 5 * time.Second,
		Wait: 10 * time.Second,
		Key: "io.vito.myServiceName",
    })
	
	go func() {
		for {
			select {
			case <- promoted:
				fmt.Println("Promoted.")
				// The application may now preform actions only a leader do
            case <-demoted:
				fmt.Println("Demoted.")
				// The application must stop preforming actions only a leader do
            case err := <-errored:
				fmt.Printf("Errored: %s\n", err)
				// When an error is encountered, the library automatically assumes
				// the leader demotion depending on its current state. This means
				// that the "demoted" channel will receive a value before an error
				// affecting this process' leader is received on this case.
			}
		}
	}()

	procLeader.Start()
	// Call Stop before exiting to allow other instance to become leader 
	// before the current TTL expires.
	defer procLeader.Stop()
}
```


## License

```
The MIT License (MIT)

Copyright (c) 2022-2023 Victor Gama

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
```
