# Go Testing Best Practices (2025)

## Table-Driven Tests

The standard pattern in Go for testing multiple scenarios:

```go
func TestSomething(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
        wantErr  bool
    }{
        {"valid input", "foo", "bar", false},
        {"empty input", "", "", true},
        {"special chars", "a@b", "a_b", false},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := DoSomething(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("unexpected error: %v", err)
            }
            if result != tt.expected {
                t.Errorf("got %v, want %v", result, tt.expected)
            }
        })
    }
}
```

### Parallel Execution

Speed up tests by running table cases in parallel:

```go
for _, tt := range tests {
    tt := tt // capture range variable (required for Go < 1.22)
    t.Run(tt.name, func(t *testing.T) {
        t.Parallel()
        // test logic
    })
}
```

Always run with race detector: `go test -race ./...`

---

## Testing CLI Applications

### Golden Files

Store expected output in files, compare against actual output:

```go
var update = flag.Bool("update", false, "update golden files")

func TestCLIOutput(t *testing.T) {
    cmd := exec.Command("./myapp", "subcommand", "--flag")
    got, err := cmd.CombinedOutput()
    if err != nil {
        t.Fatalf("command failed: %v", err)
    }

    golden := filepath.Join("testdata", t.Name()+".golden")

    if *update {
        if err := os.WriteFile(golden, got, 0644); err != nil {
            t.Fatalf("failed to update golden file: %v", err)
        }
    }

    want, err := os.ReadFile(golden)
    if err != nil {
        t.Fatalf("failed to read golden file: %v", err)
    }

    if !bytes.Equal(got, want) {
        t.Errorf("output mismatch:\ngot:\n%s\nwant:\n%s", got, want)
    }
}
```

Update golden files: `go test -update ./...`

### IO Provider Pattern

Isolate I/O for testability:

```go
type IOStreams struct {
    In     io.Reader
    Out    io.Writer
    ErrOut io.Writer
}

func NewTestIOStreams() (*IOStreams, *bytes.Buffer, *bytes.Buffer) {
    stdout := &bytes.Buffer{}
    stderr := &bytes.Buffer{}
    return &IOStreams{
        In:     strings.NewReader(""),
        Out:    stdout,
        ErrOut: stderr,
    }, stdout, stderr
}

// usage in command
type Command struct {
    io *IOStreams
}

func (c *Command) Run() error {
    fmt.Fprintln(c.io.Out, "output")
    return nil
}

// test
func TestCommand(t *testing.T) {
    io, stdout, _ := NewTestIOStreams()
    cmd := &Command{io: io}

    if err := cmd.Run(); err != nil {
        t.Fatal(err)
    }

    if got := stdout.String(); got != "output\n" {
        t.Errorf("got %q, want %q", got, "output\n")
    }
}
```

### Testing Cobra Commands

```go
func TestRootCommand(t *testing.T) {
    tests := []struct {
        name    string
        args    []string
        wantOut string
        wantErr bool
    }{
        {"help flag", []string{"--help"}, "Usage:", false},
        {"unknown flag", []string{"--unknown"}, "", true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            stdout := &bytes.Buffer{}
            stderr := &bytes.Buffer{}

            cmd := NewRootCommand()
            cmd.SetOut(stdout)
            cmd.SetErr(stderr)
            cmd.SetArgs(tt.args)

            err := cmd.Execute()
            if (err != nil) != tt.wantErr {
                t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
            }

            if tt.wantOut != "" && !strings.Contains(stdout.String(), tt.wantOut) {
                t.Errorf("output %q does not contain %q", stdout.String(), tt.wantOut)
            }
        })
    }
}
```

---

## Testing HTTP Clients

### Using httptest (Preferred)

Standard library approach - no external dependencies:

```go
func TestAPIClient_GetUser(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // verify request
        if r.Method != http.MethodGet {
            t.Errorf("expected GET, got %s", r.Method)
        }
        if r.URL.Path != "/api/users/123" {
            t.Errorf("unexpected path: %s", r.URL.Path)
        }
        if r.Header.Get("Authorization") != "Bearer token123" {
            t.Errorf("missing or invalid auth header")
        }

        // send response
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(map[string]any{
            "id":   123,
            "name": "John Doe",
        })
    }))
    defer server.Close()

    client := NewClient(server.URL, "token123")
    user, err := client.GetUser(123)

    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if user.Name != "John Doe" {
        t.Errorf("got name %q, want %q", user.Name, "John Doe")
    }
}
```

### Testing Error Scenarios

```go
func TestAPIClient_Errors(t *testing.T) {
    tests := []struct {
        name       string
        handler    http.HandlerFunc
        wantErrMsg string
    }{
        {
            name: "server error",
            handler: func(w http.ResponseWriter, r *http.Request) {
                w.WriteHeader(http.StatusInternalServerError)
                w.Write([]byte(`{"error": "internal error"}`))
            },
            wantErrMsg: "server error: 500",
        },
        {
            name: "invalid json",
            handler: func(w http.ResponseWriter, r *http.Request) {
                w.Write([]byte(`not json`))
            },
            wantErrMsg: "failed to decode response",
        },
        {
            name: "timeout",
            handler: func(w http.ResponseWriter, r *http.Request) {
                time.Sleep(2 * time.Second)
            },
            wantErrMsg: "context deadline exceeded",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            server := httptest.NewServer(tt.handler)
            defer server.Close()

            ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
            defer cancel()

            client := NewClient(server.URL, "")
            _, err := client.GetUserWithContext(ctx, 1)

            if err == nil {
                t.Fatal("expected error, got nil")
            }
            if !strings.Contains(err.Error(), tt.wantErrMsg) {
                t.Errorf("error %q does not contain %q", err.Error(), tt.wantErrMsg)
            }
        })
    }
}
```

### Interface-Based Mocking

Define interface for HTTP client to allow mocking:

```go
// HTTPClient interface for mocking
type HTTPClient interface {
    Do(req *http.Request) (*http.Response, error)
}

// API client using the interface
type Client struct {
    baseURL    string
    httpClient HTTPClient
}

func NewClient(baseURL string, httpClient HTTPClient) *Client {
    if httpClient == nil {
        httpClient = &http.Client{Timeout: 30 * time.Second}
    }
    return &Client{baseURL: baseURL, httpClient: httpClient}
}

// Mock implementation
type MockHTTPClient struct {
    DoFunc func(req *http.Request) (*http.Response, error)
}

func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
    return m.DoFunc(req)
}

// Test using mock
func TestClient_WithMock(t *testing.T) {
    mock := &MockHTTPClient{
        DoFunc: func(req *http.Request) (*http.Response, error) {
            return &http.Response{
                StatusCode: 200,
                Body:       io.NopCloser(strings.NewReader(`{"id": 1}`)),
            }, nil
        },
    }

    client := NewClient("http://api.example.com", mock)
    // test client methods
}
```

### Using httpmock Library

```go
import "github.com/jarcoal/httpmock"

func TestWithHTTPMock(t *testing.T) {
    httpmock.Activate()
    defer httpmock.DeactivateAndReset()

    // register mock responses
    httpmock.RegisterResponder("GET", "https://api.example.com/users/1",
        httpmock.NewJsonResponderOrPanic(200, map[string]any{
            "id":   1,
            "name": "Test User",
        }))

    httpmock.RegisterResponder("POST", "https://api.example.com/users",
        func(req *http.Request) (*http.Response, error) {
            // custom logic based on request
            return httpmock.NewJsonResponse(201, map[string]any{"id": 2})
        })

    // run tests against http.DefaultClient
    client := NewClient("https://api.example.com")
    user, _ := client.GetUser(1)

    // verify call count
    info := httpmock.GetCallCountInfo()
    if info["GET https://api.example.com/users/1"] != 1 {
        t.Error("expected 1 call to GET /users/1")
    }
}
```

---

## Testify

### Assertions

```go
import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestWithTestify(t *testing.T) {
    // assert continues on failure
    assert.Equal(t, expected, actual, "values should match")
    assert.NotNil(t, obj)
    assert.Error(t, err)
    assert.Contains(t, str, "substring")

    // require stops test on failure (preferred in most cases)
    require.NoError(t, err)
    require.NotNil(t, result)
}
```

**Rule of thumb:** Use `require` for setup/preconditions, `assert` for multiple independent checks.

### Test Suites

```go
import (
    "testing"
    "github.com/stretchr/testify/suite"
)

type ClientTestSuite struct {
    suite.Suite
    client *Client
    server *httptest.Server
}

func (s *ClientTestSuite) SetupSuite() {
    // runs once before all tests
}

func (s *ClientTestSuite) TearDownSuite() {
    // runs once after all tests
}

func (s *ClientTestSuite) SetupTest() {
    // runs before each test
    s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    }))
    s.client = NewClient(s.server.URL)
}

func (s *ClientTestSuite) TearDownTest() {
    // runs after each test
    s.server.Close()
}

func (s *ClientTestSuite) TestGetUser() {
    user, err := s.client.GetUser(1)
    s.Require().NoError(err)
    s.Equal("expected", user.Name)
}

func TestClientSuite(t *testing.T) {
    suite.Run(t, new(ClientTestSuite))
}
```

### Mocking with Testify

```go
import "github.com/stretchr/testify/mock"

type MockRepository struct {
    mock.Mock
}

func (m *MockRepository) GetByID(id int) (*User, error) {
    args := m.Called(id)
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).(*User), args.Error(1)
}

func TestService(t *testing.T) {
    mockRepo := new(MockRepository)
    mockRepo.On("GetByID", 1).Return(&User{ID: 1, Name: "Test"}, nil)
    mockRepo.On("GetByID", 999).Return(nil, errors.New("not found"))

    service := NewService(mockRepo)

    user, err := service.GetUser(1)
    require.NoError(t, err)
    assert.Equal(t, "Test", user.Name)

    _, err = service.GetUser(999)
    assert.Error(t, err)

    mockRepo.AssertExpectations(t)
}
```

---

## Project Structure

```
project/
├── cmd/
│   └── myapp/
│       └── main.go
├── internal/
│   ├── client/
│   │   ├── client.go
│   │   └── client_test.go
│   └── cmd/
│       ├── root.go
│       └── root_test.go
├── testdata/
│   ├── TestCLIOutput.golden
│   └── fixtures/
│       └── user.json
└── go.mod
```

### Test Fixtures

```go
func loadFixture(t *testing.T, name string) []byte {
    t.Helper()
    path := filepath.Join("testdata", "fixtures", name)
    data, err := os.ReadFile(path)
    if err != nil {
        t.Fatalf("failed to load fixture %s: %v", name, err)
    }
    return data
}

func TestParseUser(t *testing.T) {
    data := loadFixture(t, "user.json")
    user, err := ParseUser(data)
    require.NoError(t, err)
    assert.Equal(t, "John", user.Name)
}
```

---

## Key Recommendations

1. **Prefer httptest over mocking libraries** - standard library, no dependencies
2. **Use table-driven tests** - reduce duplication, cover edge cases
3. **Test error paths** - timeouts, network failures, invalid responses
4. **Use `require` over `assert`** - fail fast on critical errors
5. **Run with race detector** - `go test -race ./...`
6. **Keep tests independent** - no shared state between tests
7. **Use `t.Helper()`** - in test helper functions for better error reporting
8. **Clean up resources** - always `defer server.Close()`
9. **Use `t.Parallel()`** - where tests are independent
10. **Golden files for CLI output** - easier to maintain complex expected output

---

## Useful Commands

```bash
# run all tests
go test ./...

# run with race detector
go test -race ./...

# run with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# run specific test
go test -run TestName ./path/to/package

# verbose output
go test -v ./...

# update golden files
go test -update ./...

# run tests in parallel (default is GOMAXPROCS)
go test -parallel 4 ./...
```

---

## References

- [Go Wiki: TableDrivenTests](https://go.dev/wiki/TableDrivenTests)
- [Go testing package](https://pkg.go.dev/testing)
- [httptest package](https://pkg.go.dev/net/http/httptest)
- [testify](https://github.com/stretchr/testify)
- [httpmock](https://github.com/jarcoal/httpmock)
- [gock](https://github.com/h2non/gock)
