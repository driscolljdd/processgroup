package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/driscolljdd/processgroup"
	"time"
)

	func main() {

		// Create our new process group - it is automatically sensitive to unix quit signals and all processes stop as soon as a signal occursd
		pg := processgroup.ProcessGroup{}

		// We can also create a process group that stops if any of it's processes has an error, effectively an errgroup
		// pg := processgroup.ProcessGroup{ StopOnError: true }

		// Create some workers
		pg.Run("one", worker)
		pg.Run("two", worker)
		pg.Run("three", worker)
		pg.Run("four", worker)

		// A new feature (2019-09-19) - we now have shared (pool) channels. Start up some processes to show this off
		pg.Run("pool01", poolWorker)
		pg.Run("pool02", poolWorker)
		pg.Run("pool03", poolWorker)

		// And start a manager who will dole out work to the pool
		pg.Run("poolBoss", poolBoss)

		// We can also run any process we like that stops when it throws an error
		// pg.RunStopOnError("errorprone", worker)

		// The process group can send messages to the workers at any time.
		// Use this function now to send an initial message to one of the workers
		pg.SendMessage("three", myCustomMessage{ Sender: "Main()", Recipient: "three", Message: "This could be any struct and could contain some initial work to process"})

		// You can stop workers at any time - so let's wait three seconds and then kill off two processes
		time.Sleep(time.Second * 3)

		// As promised let's kill of processes two and four
		pg.Stop("two")
		pg.Stop("four")

		// This is the wait() function - you'll recognise from wait groups. This will pause main thread execution until all the workers end
		pg.Wait()

		// Final act, dump the log of everything that happened
		jsonBytes, _ := json.Marshal(pg.Log())
		fmt.Println(string(jsonBytes))
	}

	func worker(process processgroup.Process) {

		for {

			// This is the best way to check if the process should end - regularly consult the Alive() function. If false, then we should look to end ourselves as soon as possible.
			if(!process.Alive()) {

				// All processes can contribute easily to a central log of what happened in the entire process group
				process.Log("Ending")
				fmt.Println(process.Name, " ended")
				return
			}

			// Here we can check if anyone, the processgroup owner or another process, has sent us some post. Also see process.WaitForMessage(), the blocking equivalent of this
			raw := process.CheckForMessage()

			if(raw != nil) {

				// All channels in the processgroup are interface{}, so you can actually send anything you want through them. Cast back to the true type when you receive your messages.
				msg := raw.(myCustomMessage)
				process.Log("Got a message: " + msg.Message)
				fmt.Println(process.Name," -> Got this: ",msg)
			}

			// Processes can easily flag up errors - this causes their own context to end if they were created as RunStopOnError(), but also the process group will cease if that was set to stop on error
			// Errors automatically appear in the process group log
			process.Error(errors.New("We've had a problem"))

			// Any process can broadcast a message to the whole rest of the process group. This is not some shared channel where fastest finger gets the message; every process gets a copy in their own channel
			process.Log("Sending out a broadcast")
			process.Broadcast(myCustomMessage{ Sender: process.Name, Recipient: "Everyone", Message: "Hello world!" })

			// Any process can send a message direct to any other process in the group
			process.SendMessage("three", myCustomMessage{ Sender: process.Name, Recipient: "You", Message: "Hi there from me"})

			// Loop indefinitely doing our 'work' unless we are no longer alive
			fmt.Println(process.Name, " working")
			time.Sleep(time.Second * 1)
		}
	}

	func poolWorker(process processgroup.Process) {

		// Subscribe to a shared channel so that we can have a chance to grab incoming work - effectively become part of a worker pool
		process.Subscribe("work")

		for {

			if(!process.Alive()) {

				return
			}

			// Wait for a message to come into the shared channel - any processes subscribing to this channel get messages on a first come, first served basis
			workPacket := process.WaitForSubscribedMessage()

			if(workPacket == nil) {

				continue
			}

			fmt.Println("Worker",process.Name,"got this work packet:",workPacket)
		}
	}

	func poolBoss(process processgroup.Process) {

		for {

			if(!process.Alive()) {

				return
			}

			// Dump something into the work queue - of course this could be anything including a complex struct to be processed
			process.SendSubsriberMessage("work", "Something to do")
			time.Sleep(time.Second * 1)
		}
	}

	// This represents some complicated struct with all the payload you need to swap messages between processes
	type myCustomMessage struct {

		Sender, Recipient, Message string
	}