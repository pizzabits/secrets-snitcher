# TUI Architecture - A Guide for C/C++ Developers

> Since I'm mostly proficient in C/C++, I decided to keep a documentation for people who aren't familiar with Go. I am learning this on the Go (pun intended). If you're coming from C/C++ like me, this should help you navigate the codebase without drowning in unfamiliar patterns.

## The 30-second version

This TUI is a terminal dashboard built with [Bubble Tea](https://github.com/charmbracelet/bubbletea), which implements **The Elm Architecture** - a pattern where your entire UI is a pure function of state. 
If you've ever written a game loop in C, it's the same idea:

```
while (running) {
    event = poll_events();      // Update()
    state = process(state, event);
    render(state);              // View()
}
```

Except Bubble Tea manages the loop, concurrency, and terminal raw mode for you.

## File map


| File        | C++ equivalent                         | What it does                                                                 |
| ----------- | -------------------------------------- | ---------------------------------------------------------------------------- |
| `main.go`   | `main.cpp`                             | Entry point. Parses CLI flags, manages terminal state, starts the event loop |
| `model.go`  | Your main header with the state struct | Defines all state, message types, initialization                             |
| `update.go` | Event handler / state machine          | Processes every event and returns new state                                  |
| `view.go`   | Render function                        | Takes state, returns a string. That's it. Pure function.                     |
| `client.go` | HTTP client class                      | Fetches JSON from the API, deserializes into structs                         |
| `termbg.go` | Low-level termios code                 | Queries/sets terminal background color via escape sequences                  |


## Key Go concepts mapped to C++

### Structs and methods (no classes)

Go has no classes, no inheritance, no constructors. A struct is just a struct. Methods are functions with a receiver:

```go
// Go
type model struct {
    cursor int
    entries []Entry
}

func (m model) View() string {  // value receiver = const method
    return fmt.Sprintf("%d entries", len(m.entries))
}

func (m *model) sortEntries() { // pointer receiver = mutating method
    sort.Slice(m.entries, ...)
}
```

```cpp
// C++ equivalent
struct model {
    int cursor;
    std::vector<Entry> entries;

    std::string View() const {  // const = value receiver
        return std::format("{} entries", entries.size());
    }

    void sortEntries() {        // non-const = pointer receiver
        std::sort(entries.begin(), entries.end(), ...);
    }
};
```

**Value receiver** `(m model)` = you get a copy. Like a `const` method - you can read but changes are local. **Pointer receiver** `(m *model)` = you get the real thing. Like a regular method.

### Interfaces (duck typing)

Go interfaces are satisfied implicitly - no `implements` keyword. If your struct has the right methods, it satisfies the interface:

```go
// Bubble Tea defines this interface
type Model interface {
    Init() Cmd
    Update(Msg) (Model, Cmd)
    View() string
}

// Our model satisfies it because it has all three methods.
// No "implements" declaration needed.
```

In C++ terms, think of it like a concept/template constraint that's checked at compile time, except Go checks it at assignment time.

### Slices vs vectors

```go
entries []Entry                          // like std::vector<Entry>
entries = append(entries, newEntry)       // like push_back
entries[i]                               // same indexing
len(entries)                             // like .size()
entries[1:3]                             // like span/subrange - no copy!
```

The big gotcha: slices are **reference types**. Assigning a slice doesn't copy the data - both variables point to the same backing array. Like passing a `std::span` or a raw pointer+length.

### Maps vs unordered_map

```go
prevPods map[string]bool                 // like std::unordered_map<string, bool>
prevPods["my-pod"] = true                // insert/update
if prevPods["my-pod"] { ... }            // lookup (returns zero value if missing)
val, ok := prevPods["my-pod"]            // lookup with existence check
delete(prevPods, "my-pod")               // erase
```

Maps are also reference types (like slices). Passing a map to a function lets the function modify the original.

### Error handling (no exceptions)

```go
resp, err := c.FetchEntries()
if err != nil {
    return nil, fmt.Errorf("fetch failed: %w", err)
}
```

Go returns errors as values. No try/catch, no exceptions. Every function that can fail returns `(result, error)`. You check `err != nil` after every call. It's verbose but explicit - like checking every `errno` in C, except the compiler helps you not forget the return value.

### Goroutines and channels (concurrency)

Bubble Tea uses these internally but we don't directly. A `tea.Cmd` is a function that runs in a goroutine (lightweight thread) and returns a `tea.Msg`:

```go
func fetchData(c *Client) tea.Cmd {
    return func() tea.Msg {           // This runs in a goroutine
        resp, err := c.FetchEntries() // Blocking HTTP call
        return dataMsg{response: resp, err: err}  // Result sent back
    }
}
```

Think of it like posting a task to a thread pool in C++. The framework calls your function on a background thread, and when it returns, the result is fed back to `Update()` on the main thread. No locks needed - the architecture prevents data races by design.

## The Elm Architecture - how data flows

### 1. Init (startup)

```go
func (m model) Init() tea.Cmd {
    return tea.Batch(
        splashTimer(),           // fire splashDoneMsg in 2s
        fetchData(m.client),     // start first HTTP request
        tickCmd(m.interval),     // start 2s repeating timer
    )
}
```

`tea.Batch` is like launching multiple async tasks. All three run concurrently.

### 2. Update (event handling)

Every event in the system is a `tea.Msg`. Update receives it and returns (newState, nextCommand):

```
WindowSizeMsg  -->  store new dimensions
tickMsg        -->  fire another fetch + restart timer
dataMsg        -->  update entries, detect new pods, sort
KeyMsg         -->  handle navigation, search, sort keys
splashDoneMsg  -->  transition splash -> dashboard
goodbyeDoneMsg -->  exit program
```

The state machine is explicit. No callbacks, no observers, no event buses. Just a big switch statement. If you've written a `select`/`poll` event loop in C, this is the structured version of that.

### 3. View (rendering)

`View()` is a **pure function**. Given the same model state, it always produces the same output. It never modifies state. It returns one big string that Bubble Tea prints to the terminal.

```go
func (m model) View() string {
    switch m.phase {
    case phaseSplash:
        return m.viewSplash()    // raccoon + title
    case phaseGoodbye:
        return m.viewGoodbye()   // farewell screen
    }
    return m.viewDashboard()     // the main table
}
```

Styling uses two layers:

- **lipgloss** - row-level background/foreground (like ncurses attributes)
- **Raw ANSI** - inline cell coloring (`\033[38;2;R;G;Bm` - you know these from terminal programming)

### Why two layers?

lipgloss sets a background color per row. But individual cells within a row need different foreground colors (red for anomalies, green for cached). Raw ANSI escape codes handle that without breaking lipgloss's background fill.

## Terminal background management (termbg.go)

This file will feel very familiar if you've done terminal programming in C:

```go
// 1. Get current terminal attributes (like tcgetattr)
orig, err := unix.IoctlGetTermios(fd, unix.TIOCGETA)

// 2. Switch to raw mode (no echo, no canonical processing)
raw.Lflag &^= unix.ECHO | unix.ICANON

// 3. Send OSC 11 query to ask terminal its background color
fmt.Print("\033]11;?\033\\")

// 4. Read response with timeout
// 5. Restore original terminal attributes (like tcsetattr)
```

The `&^=` operator is Go's "bit clear" - equivalent to `&= ~(ECHO | ICANON)` in C.

## Testing approach

Tests exercise the model directly without needing a terminal:

```go
func TestNavigationKeys(t *testing.T) {
    m := testModel(sampleEntries())  // create model with test data
    m.cursor = 0

    updated, _ := m.Update(tea.KeyMsg{...})  // simulate keypress
    result := updated.(model)                 // type assert back to model

    if result.cursor != 1 { ... }            // verify state changed
}
```

The `.(model)` syntax is a **type assertion** - like `dynamic_cast<model>` in C++. Since `Update` returns the interface type `tea.Model`, we assert it back to our concrete `model` to inspect fields.

## Quick reference


| Go                      | C++                                |
| ----------------------- | ---------------------------------- |
| `func (m model) Foo()`  | `void Foo() const`                 |
| `func (m *model) Foo()` | `void Foo()`                       |
| `[]Entry`               | `std::vector<Entry>`               |
| `map[string]bool`       | `std::unordered_map<string, bool>` |
| `interface{}`           | `std::any` / `void`*               |
| `err != nil`            | checking errno / exceptions        |
| `go func(){}()`         | `std::async(...)`                  |
| `defer cleanup()`       | RAII / scope guard                 |
| `:=`                    | `auto x =`                         |
| `&^=`                   | `&= ~`                             |
