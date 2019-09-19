package processgroup

import (
	"golang.org/x/net/context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
	"reflect"
)

type processAction func(process Process)



	type ProcessGroup struct {

		context context.Context
		cancel context.CancelFunc
		channel chan groupMessage
		group sync.WaitGroup
		processes map[string]Process
		setupComplete bool
		lock sync.RWMutex
		log []struct{ Date time.Time; Process, Event string }
		StopOnError bool
		workChannels map[string]chan interface{}
	}

	func (pg *ProcessGroup) setup() {

		if(pg.setupComplete) {

			return
		}

		// Set up the top level context - the group context
		pg.context, pg.cancel = context.WithCancel(context.Background())

		// The channel to facilitate messaging between processes
		pg.channel = make(chan groupMessage, 1000)

		// An audit log for the process group
		pg.log = make([]struct{ Date time.Time; Process, Event string }, 0)

		// Initialise our processes map
		pg.processes = make(map[string]Process)

		// Initialise our map of work channels
		pg.workChannels = make(map[string]chan interface{})

		// Watch for unix signals
		go func() {

			// Make a channel to receive unix signals
			signalChannel := make(chan os.Signal, 1)

			// Register for notifications of being asked to quit by unix
			signal.Notify(signalChannel, syscall.SIGINT, syscall.SIGTERM)

			// Wait for a quit instruction from outside
			<- signalChannel

			// Call the cancel function for our process group
			pg.cancel()
		}()

		// Hire a post person to watch the group mailbox
		pg.group.Add(1)
		go func() {

			defer pg.group.Done()

			// Loop while waiting for post
			alive := true
			for {

				select {

					case <- pg.context.Done():

						alive = false

					case msg := <- pg.channel:

						if(msg.Log) {

							pg.log = append(pg.log, struct{ Date time.Time; Process, Event string }{Date: time.Now(), Process: msg.Sender, Event: msg.Message.(string)})

						} else if(msg.Error) {

							pg.log = append(pg.log, struct{ Date time.Time; Process, Event string }{Date: time.Now(), Process: msg.Sender, Event: "Error: " + msg.Message.(error).Error() })

							if(pg.StopOnError) {

								// If instructed, shut down the whole process group on error
								pg.cancel()
							}

						} else if(msg.Broadcast) {

							// Loop through every process and send this message
							pg.lock.RLock()

							for name, process := range pg.processes {

								// Don't send a message back to the sender
								if(name == msg.Sender) {

									continue
								}

								select {

									case process.channel <- msg.Message:
									default:
								}
							}

						} else if(msg.Subscriber) {

							pg.lock.RLock()

							channel, exists := pg.workChannels[msg.Subject]

							pg.lock.RUnlock()

							if(exists) {

								select {

									case channel <- msg.Message:
									default:
								}
							}

						} else {

							// Find the recipient for this message
							process, found := pg.getProcess(msg.Recipient)

							if(found) {

								select {

									case process.channel <- msg.Message:
									default:
								}
							}
						}
				}

				if(!alive) {

					break
				}
			}
		}()

		// Done
		pg.setupComplete = true
	}

	// Start up a new process. Give it a name, and the second parameter is a function with this definition -> func <funcname>(process processgroup.Process)
	func (pg *ProcessGroup) Run(name string, action processAction) {

		// With every public method calling setup, we avoid the need for a constructor function and it becomes safe
		// to allow end users to be able to acess the process group itself (ProcessGroup instead of processGroup)
		pg.setup()

		// Build a process for this
		myProcess := Process{ Name: name }
		myProcess.context, myProcess.cancel = context.WithCancel(pg.context)
		myProcess.channel = make(chan interface{}, 1)
		myProcess.group_channel = pg.channel
		myProcess.subscribed_channels = make([]chan interface{}, 0)

		// To allow processes to phone home and set up work channels, include a link back to the parent
		myProcess.parent = pg

		// Register this process in our collection
		pg.lock.Lock()
		pg.processes[name] = myProcess
		pg.lock.Unlock()

		// Run the action function
		pg.group.Add(1)
		go func() {

			// Handle the wait group
			defer pg.group.Done()

			// Run the actual payload
			action(myProcess)
		}()
	}

	func (pg *ProcessGroup) RunStopOnError(name string, action processAction) {

		// With every public method calling setup, we avoid the need for a constructor function and it becomes safe
		// to allow end users to be able to acess the process group itself (ProcessGroup instead of processGroup)
		pg.setup()

		// Build a process for this
		myProcess := Process{ Name: name, stop_on_error: true }
		myProcess.context, myProcess.cancel = context.WithCancel(pg.context)
		myProcess.channel = make(chan interface{}, 1)
		myProcess.group_channel = pg.channel
		myProcess.subscribed_channels = make([]chan interface{}, 0)

		// Register this process in our collection
		pg.lock.Lock()
		pg.processes[name] = myProcess
		pg.lock.Unlock()

		// Run the action function
		pg.group.Add(1)
		go func() {

			// Handle the wait group
			defer pg.group.Done()

			// Run the actual payload
			action(myProcess)
		}()
	}

	func (pg *ProcessGroup) Stop(name string) {

		// With every public method calling setup, we avoid the need for a constructor function and it becomes safe
		// to allow end users to be able to acess the process group itself (ProcessGroup instead of processGroup)
		pg.setup()

		// Find the named process in our map of processes
		process, found := pg.getProcess(name)

		if(!found) {

			return
		}

		// Cancel the context for this process
		process.cancel()
	}

	// The wait in wait group - block until all processes are complete
	func (pg *ProcessGroup) Wait() {

		// With every public method calling setup, we avoid the need for a constructor function and it becomes safe
		// to allow end users to be able to acess the process group itself (ProcessGroup instead of processGroup)
		pg.setup()

		// Use the wait function of our wait group
		pg.group.Wait()
	}

	// Allow the main() function or whoever owns the process group struct, to send message(s) to member processes of the group
	// Not for me to say how to use this but could be a fine way to send a complex struct to a process after creation, containing a packet of work to process
	func (pg *ProcessGroup) SendMessage(to string, message interface{}) {

		// With every public method calling setup, we avoid the need for a constructor function and it becomes safe
		// to allow end users to be able to acess the process group itself (ProcessGroup instead of processGroup)
		pg.setup()

		// Locate the recipient process
		process, found := pg.getProcess(to)

		if(!found) {

			return
		}

		select {

			case process.channel <- message:
			default:
		}
	}

	// Allow the main() function or the owner of the process group to send a message to a channel which processes might subscribe to. It's like a pool of workers, so
	// if five processes subscribe to a 'tasks' channel, any message going down that channel will be grabbed by the first process to be ready to receive
	//
	// Although this function is provided I would say don't use it. If you start doing significant work in your main() thread, you will need to watch for unix kill signals
	// because the main() is outside of this process group. A better design pattern is to set everything up and put everything you want your app to do into processes, and then
	// have the main thread just call ProcessGroup.Wait() and hang around until all processes end (which they will on unix kill because this library is taking care of it for you).
	func (pg *ProcessGroup) SendSubscriberMesssage(subject string, message interface{}) bool {

		// Avoid needing a constructor by putting this at the top of all public methods
		pg.setup()

		pg.lock.RLock()
		channel, exists := pg.workChannels[subject]
		pg.lock.RUnlock()

		if(!exists) {

			return false
		}

		// Send this message if it's possible to do so, otherwise return false
		select {

			case channel <- message:

				return true

			default:
		}

		return false
	}

	func (pg *ProcessGroup) Log() []struct{ Date time.Time; Process, Event string } {

		// With every public method calling setup, we avoid the need for a constructor function and it becomes safe
		// to allow end users to be able to acess the process group itself (ProcessGroup instead of processGroup)
		pg.setup()

		// Get the log of events
		return pg.log
	}

	// The 'all stop' command - stop every process
	func (pg *ProcessGroup) End() {

		// With every public method calling setup, we avoid the need for a constructor function and it becomes safe
		// to allow end users to be able to acess the process group itself (ProcessGroup instead of processGroup)
		pg.setup()

		// Kill our group context - all process contexts are children of that
		pg.cancel()
	}

	func (pg *ProcessGroup) getProcess(name string) (Process, bool) {

		pg.lock.RLock()
		process, found := pg.processes[name]
		pg.lock.RUnlock()

		if(!found) {

			return Process{}, false
		}

		return process, true
	}

	func (pg *ProcessGroup) subscribe(subject string) chan interface{} {

		// Does this channel exist?
		pg.lock.RLock()
		channel, exists := pg.workChannels[subject]
		pg.lock.RUnlock()

		if(!exists) {

			// Let's create this
			channel = make(chan interface{}, 100)

			pg.lock.Lock()
			pg.workChannels[subject] = channel
			pg.lock.Unlock()
		}

		return channel
	}



	type Process struct {

		Name string
		context context.Context
		cancel context.CancelFunc
		channel chan interface{}
		group_channel chan groupMessage
		subscribed_channels []chan interface{}
		stop_on_error bool
		parent *ProcessGroup
	}

	// An easy to use function for end users to check if a process is still, for want of a better word, "alive"
	func (p *Process) Alive() bool {

		select {

			case <- p.context.Done():

				return false

			default:

				return true
		}
	}

	// Scan quickly to see if a message has come in (non blocking)
	func (p *Process) CheckForMessage() interface{} {

		select {

			case msg := <- p.channel:

				return msg

			case <- p.context.Done():

				return nil

			default:

				return nil
		}
	}

	// Block your process and wait for the arrival of a message
	func (p *Process) WaitForMessage() interface{} {

		// OK secretly, don't actually wait for ever - the context is king so if we are done(), then stop
		select {

			case <- p.context.Done():

				return nil

			case msg := <- p.channel:

				return msg
		}
	}

	// Wait for something from our subscribed work channels
	func (p *Process) WaitForSubscribedMessage() interface{} {

		var cases []reflect.SelectCase
		for _, ch := range p.subscribed_channels {
			cases = append(cases, reflect.SelectCase{
				Dir:  reflect.SelectRecv,
				Chan: reflect.ValueOf(ch),
				Send: reflect.Value{},
			})
		}

		// Add in our instruction to die so we die in a timely fashion when required
		cases = append(cases, reflect.SelectCase{ Dir: reflect.SelectRecv, Chan: reflect.ValueOf(p.context.Done()), Send: reflect.Value{} })

		// Get a subscribed message
		_, val, ok := reflect.Select(cases)

		if(ok) {

			return val
		}

		return nil
	}

	// This is cool - allow the end user to message any other process directly from their individual processes
	func (p *Process) SendMessage(to string, message interface{}) {

		// If you're sending a message to yourself that's probably a coding mistake or whatever but it makes no sense. Kindly just edit that out for the end user
		if(p.Name == to) {

			return
		}

		// Send this back to control for the post person to intercept
		msg := groupMessage{ Sender: p.Name, Recipient: to, Message: message }

		// Make all send operations non blocking, so we're ready for whatever the end coder comes up with
		select {

		case p.group_channel <- msg:
		default:
		}
	}

	// And here, allow the end user to message all the processes in the group - a dead simple broadcast
	func (p *Process) Broadcast(message interface{}) {

		// Send a message to control, asking for a broadcast
		msg := groupMessage{ Sender: p.Name, Broadcast: true, Message: message }

		// Make all send operations non blocking, so we're ready for whatever the end coder comes up with
		select {

			case p.group_channel <- msg:
			default:
		}
	}

	// Subscribe to a channel which multiple processes can join - the idea being you have some work three or four processes can do depending on whoever is free, and you subscribe
	// them all to a channel and then pipe a packet of work down that shared channel. Messages for a shared channel just arrive via the CheckForMessage() and WaitForMessage() functions
	func (p *Process) Subscribe(subject string) {

		// Pass this back to the parent
		p.subscribed_channels = append(p.subscribed_channels, p.parent.subscribe(subject))
	}

	// Allow processes to contribute to a group log of what's been going on
	func (p *Process) Log(event string) {

		msg := groupMessage{ Sender: p.Name, Log: true, Message: event }

		// Keep everything here non blocking, so things are very thread safe
		select {

			case p.group_channel <- msg:
			default:
		}
	}

	// Allow processes to send a message to a subscriber channel (a channel multiple processes are watching and messages are taken off first come, first served)
	func (p *Process) SendSubsriberMessage(subject string, message interface{}) {

		// Send a message to control, asking for a subscriber message to be distributed
		msg := groupMessage{ Sender: p.Name, Subscriber: true, Subject: subject, Message: message }

		// Make all send operations non blocking, so we're ready for whatever the end coder comes up with
		select {

			case p.group_channel <- msg:
			default:
		}
	}

	func (p *Process) Error(err error) {

		// Prepare a message that this is an error
		msg := groupMessage{ Sender: p.Name, Error: true, Message: err }

		// Keep everything here non blocking, so things are very thread safe
		select {

			case p.group_channel <- msg:
			default:
		}

		// If this process is supposed to stop on error, take care of that now
		if(p.stop_on_error) {

			p.cancel()
		}
	}


	// Internal message structure for control messages which will be routed around the processes
	type groupMessage struct {

		Sender, Recipient, Subject string
		Broadcast, Log, Error, Subscriber bool
		Message interface{}
	}