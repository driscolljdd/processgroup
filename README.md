# Advanced Process Groups

This library allows you to triviallly create a Process Group - a collection of goroutines which you can extert advanced levels of
control over with the minimum of code. It is really a very smart way to manage sub processes within your application.

## Features

- Start and stop processes with a single line of code.
- Easily send messages to your processes from main, from your processes to other processes or broadcast from any process to all
processes.
- Universal interface{} channel for every process can transmit any format of message.
- Easily keep a log of every event in every process in your group with a single line of code.
- Make a single process stop on error, or even the whole process group stop on an error in any of it's processes.
- Simple, well documented code which you can adapt to perfectly fit your needs
- Thread safe; the manager struct could be declared globally and all processes could be used to start / stop other processes
- Ability to subscribe processes to channels where messages are received on a first come / first served basis i.e. a pool of workers

There is no need to start a wait group or call a Done() function when your processes end; simply return out of the process function.

## Installation

Install:
```shell
go get -u github.com/driscolljdd/processgroup
```

Import:

```go
import "github.com/driscolljdd/processgroup"
```

## Quick Start

Here's an example of how to start a process group. See the Howto for a much richer example.
```go
func main() {

	pg := processgroup.ProcessGroup()
	pg.Run("one", worker)
	pg.Run("two", worker)
	pg.Run("three", worker)
	
	pg.Wait()
}

func worker(process processgroup.Process) {
	
	process.log("Started up")
	
	for {
		
		if(!process.Alive()) {
			
			return
		}
		
		msg := process.CheckForMessage()
		
		if(message != nil) {
			
			fmt.Println(process.Name," got a message: ",msg)
		}
		
		process.Broadcast("Hi from: " + process.Name)
		time.Sleep(time.Second * 1)
	}
}
````

## Howto

Please view the Example/Example.go file in this package for an easy to follow simple application which handles multiple threads.