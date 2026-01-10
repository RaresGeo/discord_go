# Go Refactoring Guide: Discord Bot Improvements

This document explains the Go-specific improvements made to the Discord bot codebase. Each section describes what was changed, why it's better, and what Go concepts are being used.

## Table of Contents
1. [Context for Lifecycle Management](#1-context-for-lifecycle-management)
2. [Heartbeat with Ticker and Channels](#2-heartbeat-with-ticker-and-channels)
3. [Thread-Safe State with sync.Mutex](#3-thread-safe-state-with-syncmutex)
4. [Error Handling Instead of os.Exit](#4-error-handling-instead-of-osexit)
5. [Resource Cleanup with defer](#5-resource-cleanup-with-defer)
6. [Buffered Channels](#6-buffered-channels)
7. [Error Wrapping with %w](#7-error-wrapping-with-w)
8. [HTTP Client Configuration](#8-http-client-configuration)
9. [Separate Reader Goroutine](#9-separate-reader-goroutine)
10. [Graceful Shutdown](#10-graceful-shutdown)

---

## 1. Context for Lifecycle Management

### Before
```go
func (c *Client) ConnectToGateway() {
    // No way to cancel or shutdown
    // Would run forever or crash
}
```

### After
```go
func (c *Client) ConnectToGateway(ctx context.Context) error {
    // Can be cancelled via context
    select {
    case <-ctx.Done():
        return ctx.Err()
    // ...
    }
}
```

### Why This is Better
- **Graceful Shutdown**: Context allows you to signal shutdown to all goroutines
- **Cancellation Propagation**: Parent contexts can cancel child operations
- **Timeout Support**: Can add timeouts without changing function signatures
- **Standard Pattern**: This is the idiomatic Go way to manage lifecycle

### Key Concepts
- `context.Context` is Go's way to pass cancellation signals and deadlines
- `context.WithCancel()` creates a cancellable context
- Always check `<-ctx.Done()` in long-running operations
- Pass context as the first parameter by convention

---

## 2. Heartbeat with Ticker and Channels

### Before (JavaScript-style)
```go
func (c *Client) setHeartbeatInterval(timeToWait int) {
    c.heartbeatTimer = time.AfterFunc(time.Duration(timeToWait)*time.Millisecond, func() {
        c.sendHeartbeat()
        c.setHeartbeatInterval(timeToWait) // Recursive!
    })
}
```

### After (Idiomatic Go)
```go
func (c *Client) startHeartbeat(ctx context.Context) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            c.sendHeartbeat()
        }
    }
}
```

### Why This is Better
- **No Recursion**: Tickers are designed for periodic tasks
- **Resource Management**: `defer ticker.Stop()` prevents resource leaks
- **Cancellable**: Respects context cancellation immediately
- **Channel-Based**: Uses Go's concurrency primitives (channels)
- **More Efficient**: No function call overhead from recursion

### Key Concepts
- `time.Ticker` sends values on a channel at regular intervals
- `select` statement multiplexes multiple channel operations
- Channels are Go's way of communicating between goroutines
- "Don't communicate by sharing memory; share memory by communicating"

---

## 3. Thread-Safe State with sync.Mutex

### Before
```go
type Client struct {
    lastHeartbeatAcked bool
    sequence           int64
    // Accessed from multiple goroutines - RACE CONDITION!
}
```

### After
```go
type Client struct {
    mu sync.RWMutex  // Protects fields below

    lastHeartbeatAcked bool
    sequence           int64
}

func (c *Client) sendHeartbeat() error {
    c.mu.Lock()
    if !c.lastHeartbeatAcked {
        c.mu.Unlock()
        return fmt.Errorf("...")
    }
    c.lastHeartbeatAcked = false
    seq := c.sequence
    c.mu.Unlock()
    // ...
}
```

### Why This is Better
- **No Race Conditions**: Multiple goroutines can safely access shared state
- **Data Integrity**: Prevents corrupted state from concurrent access
- **RWMutex Optimization**: Multiple readers can read simultaneously (RLock)
- **Detectable**: `go run -race` would catch the previous bugs

### Key Concepts
- `sync.Mutex` provides mutual exclusion (only one goroutine at a time)
- `sync.RWMutex` allows multiple readers OR one writer
- Use `Lock()`/`Unlock()` for writes, `RLock()`/`RUnlock()` for reads
- Always `defer mu.Unlock()` to prevent deadlocks
- Group the mutex with the fields it protects in the struct

### Common Pattern
```go
// Reading
c.mu.RLock()
value := c.someField
c.mu.RUnlock()

// Writing
c.mu.Lock()
defer c.mu.Unlock()
c.someField = newValue
```

---

## 4. Error Handling Instead of os.Exit

### Before
```go
func NewBot(token string) *Client {
    res, err := http.DefaultClient.Do(req)
    if err != nil {
        fmt.Printf("error: %s\n", err)
        os.Exit(1)  // Kills entire program!
    }
    return &Client{...}
}
```

### After
```go
func NewBot(token string, prefix string) (*Client, error) {
    res, err := httpClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("error making http request: %w", err)
    }
    return &Client{...}, nil
}
```

### Why This is Better
- **Caller Control**: Caller decides how to handle errors
- **Testable**: Can test error cases without killing test process
- **Composable**: Functions can be used in libraries
- **Cleanup**: Deferred cleanup functions run when returning errors
- **Context**: Errors can be wrapped with additional context

### Key Concepts
- Functions that can fail should return `error` as last return value
- Check errors immediately: `if err != nil { return err }`
- Never ignore errors (Go's `_` is for intentional ignoring only)
- `os.Exit()` is only for `main()` or truly unrecoverable errors
- Use `fmt.Errorf()` to create errors with context

---

## 5. Resource Cleanup with defer

### Before
```go
res, err := http.DefaultClient.Do(req)
if err != nil {
    return err
}
// Forgot to close! Resource leak!
resBody, err := io.ReadAll(res.Body)
```

### After
```go
res, err := httpClient.Do(req)
if err != nil {
    return nil, err
}
defer res.Body.Close()  // Always executes when function returns

resBody, err := io.ReadAll(res.Body)
```

### Why This is Better
- **No Resource Leaks**: Connection is always closed
- **Guaranteed Cleanup**: Runs even if function panics
- **Clear Intent**: Cleanup code is next to allocation
- **Multiple defers**: Stack up in LIFO order

### Key Concepts
- `defer` schedules a function call to run when the surrounding function returns
- Deferred calls execute in LIFO order (last defer runs first)
- Common uses: closing files, unlocking mutexes, releasing resources
- Place `defer` immediately after acquiring a resource

### Common Patterns
```go
// Files
f, err := os.Open("file.txt")
if err != nil { return err }
defer f.Close()

// Mutexes
c.mu.Lock()
defer c.mu.Unlock()

// HTTP Responses
res, err := client.Do(req)
if err != nil { return err }
defer res.Body.Close()
```

---

## 6. Buffered Channels

### Before
```go
c.messageChannel = make(chan []byte)  // Unbuffered

// Writer blocks if reader isn't ready
c.messageChannel <- messageBody  // Can deadlock!
```

### After
```go
c.messageChannel = make(chan []byte, 10)  // Buffered

// Writer can send 10 messages before blocking
c.messageChannel <- messageBody  // Less blocking
```

### Why This is Better
- **Decoupling**: Sender and receiver don't need to be synchronized
- **Performance**: Reduces goroutine blocking and context switching
- **Buffering**: Handles bursts of messages
- **Prevents Deadlocks**: Gives room for asynchronous operation

### Key Concepts
- Unbuffered channels: `make(chan T)` - sender blocks until receiver reads
- Buffered channels: `make(chan T, size)` - sender blocks only when full
- Buffer size is a tradeoff: too small = blocking, too large = memory
- Use unbuffered for synchronization, buffered for data flow

### Choosing Buffer Size
```go
// Unbuffered - tight synchronization
done := make(chan struct{})

// Small buffer - occasional bursts
messages := make(chan Message, 10)

// Large buffer - producer/consumer
workQueue := make(chan Job, 1000)
```

---

## 7. Error Wrapping with %w

### Before
```go
if err != nil {
    return fmt.Errorf("could not connect: %s", err)
    // Original error is lost - can't check error type!
}
```

### After
```go
if err != nil {
    return fmt.Errorf("could not connect: %w", err)
    // Error chain preserved - can use errors.Is/As
}
```

### Why This is Better
- **Error Chain**: Preserves the original error
- **Type Checking**: Can use `errors.Is()` and `errors.As()`
- **Debugging**: Full error context when debugging
- **Sentinel Errors**: Can check for specific errors up the call stack

### Key Concepts
- `%w` wraps an error, preserving it in the error chain
- `%s` converts error to string, losing the original error
- `errors.Is(err, target)` checks if any error in chain matches target
- `errors.As(err, &target)` extracts an error of a specific type

### Example Usage
```go
// Creating errors
var ErrNotFound = errors.New("not found")

func GetUser(id string) (*User, error) {
    user, err := db.Query(id)
    if err != nil {
        return nil, fmt.Errorf("get user %s: %w", id, err)
    }
    return user, nil
}

// Checking errors
user, err := GetUser("123")
if errors.Is(err, sql.ErrNoRows) {
    // Handle not found
}
```

---

## 8. HTTP Client Configuration

### Before
```go
res, err := http.DefaultClient.Do(req)
// Uses default: no timeout, shared state
```

### After
```go
httpClient := &http.Client{
    Timeout: 30 * time.Second,
}
res, err := httpClient.Do(req)
```

### Why This is Better
- **Timeouts**: Prevents hanging on slow/dead connections
- **Configuration**: Can customize per use case
- **Reusable**: Reuses TCP connections (connection pooling)
- **Testable**: Can inject mock clients

### Key Concepts
- `http.DefaultClient` has no timeout - can hang forever!
- Custom clients allow timeouts, custom transports, etc.
- HTTP clients are safe for concurrent use
- Clients reuse connections via connection pooling

### Advanced Configuration
```go
client := &http.Client{
    Timeout: 30 * time.Second,
    Transport: &http.Transport{
        MaxIdleConns:        100,
        MaxIdleConnsPerHost: 10,
        IdleConnTimeout:     90 * time.Second,
    },
}
```

---

## 9. Separate Reader Goroutine

### Before
```go
func (c *Client) startListening() {
    for {
        _, msg, err := c.connection.ReadMessage()
        // Reading and processing in same goroutine
        c.handleMessage(msg)
    }
}
```

### After
```go
func (c *Client) startListening(ctx context.Context) error {
    // Reader goroutine - only reads
    go func() {
        defer close(c.messageChannel)
        for {
            _, msg, err := conn.ReadMessage()
            select {
            case c.messageChannel <- msg:
            case <-ctx.Done():
                return
            }
        }
    }()

    // Main goroutine - only processes
    for msg := range c.messageChannel {
        c.handleMessage(msg)
    }
}
```

### Why This is Better
- **Separation of Concerns**: Reading and processing are independent
- **Non-Blocking**: Reader doesn't block on slow processing
- **Cancellable**: Can cleanly shutdown via context
- **Buffering**: Channel buffers between reader and processor

### Key Concepts
- One goroutine per blocking I/O operation
- Use channels to communicate between goroutines
- `defer close(channel)` signals completion to receivers
- `for msg := range ch` idiom receives until channel closes

---

## 10. Graceful Shutdown

### Before
```go
func main() {
    bot := client.NewBot(token)
    bot.ConnectToGateway()
    // Program just runs until killed
}
```

### After
```go
func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Handle OS signals
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
    go func() {
        <-sigCh
        log.Println("Shutting down gracefully...")
        cancel()
    }()

    bot.ConnectToGateway(ctx)
}
```

### Why This is Better
- **Clean Shutdown**: Goroutines can finish current work
- **Resource Cleanup**: Deferred cleanups execute
- **User Experience**: Can save state, close connections properly
- **Production Ready**: Required for proper deployment

### Key Concepts
- `os/signal` package handles OS signals (Ctrl+C, etc.)
- `signal.Notify()` sends signals to a channel
- Context cancellation propagates to all operations
- Buffered signal channel (size 1) prevents missing signals

### Complete Pattern
```go
func main() {
    // Create cancellable context
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Handle shutdown signals
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

    // Start signal handler
    go func() {
        <-sigCh
        log.Println("Received shutdown signal")
        cancel()
    }()

    // Run application (blocks until ctx.Done())
    if err := app.Run(ctx); err != nil {
        log.Fatalf("Application error: %v", err)
    }

    log.Println("Shutdown complete")
}
```

---

## Summary of Changes

| Improvement | Go Concept | Benefit |
|------------|------------|---------|
| Context | `context.Context` | Graceful shutdown, cancellation |
| Heartbeat Ticker | `time.Ticker`, `select` | Proper periodic tasks |
| Mutex | `sync.RWMutex` | Thread-safe state access |
| Error Returns | `error` return value | Testable, composable |
| Defer Close | `defer` | No resource leaks |
| Buffered Channels | `make(chan T, n)` | Better performance |
| Error Wrapping | `fmt.Errorf("%w")` | Error chain preservation |
| HTTP Client | Custom `http.Client` | Timeouts, configuration |
| Reader Goroutine | Separate goroutines | Non-blocking I/O |
| Signal Handling | `os/signal` | Graceful shutdown |

---

## Additional Go Best Practices Applied

### Unexported Functions
Functions that are only used internally are now lowercase (unexported):
- `identify()` instead of `Identify()`
- `sendHeartbeat()` instead of `SendHeartbeat()`

This is Go's encapsulation mechanism - only exported (uppercase) names are accessible from other packages.

### Documentation Comments
All exported types and functions now have documentation comments:
```go
// NewBot creates a new Discord bot client with the given token and command prefix.
func NewBot(token string, prefix string) (*Client, error)
```

### Type Conversion
Changed from `int` milliseconds to `time.Duration`:
```go
// Before
heartbeatInterval int  // Unclear: ms? seconds?

// After
heartbeatInterval time.Duration  // Type-safe duration
```

---

## Learning Resources

To deepen your understanding of these Go patterns:

1. **Effective Go**: https://go.dev/doc/effective_go
2. **Go Concurrency Patterns**: https://go.dev/blog/pipelines
3. **Context Package**: https://go.dev/blog/context
4. **Error Handling**: https://go.dev/blog/error-handling-and-go
5. **Defer, Panic, Recover**: https://go.dev/blog/defer-panic-and-recover

---

## Running the Code

The refactored code maintains the same external API for the most part, with these changes:

```bash
# Build
go build ./...

# Run (requires .env file with DISCORD_TOKEN)
go run src/main.go

# Test for race conditions
go run -race src/main.go

# Clean shutdown with Ctrl+C
# (Will now gracefully close connections)
```

The bot will now:
- Gracefully shutdown on Ctrl+C
- Properly clean up all resources
- Log errors instead of crashing
- Handle concurrent operations safely
