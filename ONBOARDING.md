## So you want to write Complement tests

If you've not run Complement tests yet, please do so. This document will outline how Complement works and how you can add efficient tests and best practices for Complement itself.

### Architecture

Complement runs Docker containers every time you call `deployment := must.Deploy(...)`, which gets killed on `deployment.Destroy(...)`. These containers are snapshots of the target Homeserver at a particular state. The state is determined by the `Blueprint`, which is a human-readable outline for what should be done prior to the test. Coming from Sytest, a `Blueprint` is similar to a `fixture`. A `deployment` has functions on it for creating deployment-scoped structs such as CSAPI clients for interacting with specific Homeservers in the deployment. Assertions are done via the `must` and `match` packages. For testing outbound federation, Complement implements a bare-bones Federation server for Homeservers to talk to. Unlike Sytest, you have to explicitly opt-in to attaching core functionality to the server so the reader can clearly see what is and is not being handled automatically. This is done using [functional options](https://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis) and looks something like:
```go
// A federation server which handles serving up its own keys when requested,
// automatically accepts make_join and send_join requests and deals with
// room alias lookups.
srv := federation.NewServer(t, deployment,
    federation.HandleKeyRequests(),
    federation.HandleMakeSendJoinRequests(),
    federation.HandleDirectoryLookups(),
)
// begin listening on a goroutine
cancel := srv.Listen()
defer cancel()
```

Network topology for all forms of communication look like this:
```
+------+                  Outbound federation           +-----------+             Network: one per blueprint                +-----------+
|      | :35372 <------ host.docker.internal:35372 ---- |           |                                                       |           |
| Host |                                                | Container | ------- SS API https://hs2/_matrix/federation/... --> | Container |
|      | -------CS API http://localhost:38295 --------> |   (hs1)   |                                                       |   (hs2)   |
|      | ------SS API https://localhost:37562 --------> |           |                                                       |           |
+------+                                                +-----------+                                                       +-----------+
                                             docker -p 38295:8008 -p 37562:8448

The high numbered ports are randomly chosen, and are for illustrative purposes only.
```
The mapping of `hs1` to `localhost:port` combinations can be done automatically using a `docker.RoundTripper`.

### How do I...

Get a CS API client:
```go
// the user and hs name are from the blueprint
// automatically maps localhost:12345 to the right container
alice := deployment.Client(t, "hs1", "@alice:hs1")
```

Make a Federation server:
```go
srv := federation.NewServer(t, deployment,
    federation.HandleKeyRequests(),
    federation.HandleMakeSendJoinRequests(),
)
cancel := srv.Listen()
defer cancel()
```

Get a Federation client:
```go
// Federation servers sign their requests, so you need a server before
// you can make a client.
// automatically accepts self-signed TLS certs
// automatically maps localhost:12345 to the right container
// automatically signs requests
// srv == federation.Server
fedClient := srv.FederationClient(deployment, "hs1")
```

### FAQ

#### How should I name the test files / test functions?

There is a particular naming convention so that `sytest_coverage.go` can automatically pick up when tests have been added.

You have to have `_test.go` on them else Go won't run the tests in that file. Other than that, if it's copying across sytest tests then remove `/` and digits and use an `_` separator e.g `tests/30rooms/20typing.pl` becomes `rooms_typing_test.go`, `tests/52user-directory/01public.pl` becomes `user_directory_public_test.go`, `tests/44account_data.pl` becomes `account_data_test.go`, etc. When you convert a test, make it a subtest with the perl test name e.g:

```go
func TestFoo(t *testing.T) {
    t.Run("Outbound federation can query profile data", func(t *testing.T) {
        // ....
    })
}
```

This slightly awkward structure means `sytest_coverage.go` will know the test is converted and automatically update the list when run! If you are struggling to get the script to pick up the file, just do `go run sytest_coverage.go -v` to see the exact filename and expected string.

#### Should I always make a new blueprint for a test?

Probably not. Blueprints are costly, and they should only be made if there is a strong case for plenty of reuse among tests. In the same way that we don't always add fixtures to sytest, we should be sparing with adding blueprints.

#### How should I assert JSON objects?

Use one of the matchers in the `match` package (which uses `gjson`) rather than `json.Unmarshal(...)` into a struct. There's a few reasons for this:
 - Removes the temptation to use `gomatrixserverlib` structs.
 - Forces exact key matching (without JSON tags, `json.Unmarshal` will case-insensitively match fields by default).
 - Clearer syntax: `match.JSONKeyEqual("earliest_events", []interface{}{latestEvent.EventID()}),` is clearer than unmarshalling into a slice then doing assertions on the first element.

If you want to extract data from objects, just use `gjson` directly.

#### How should I assert HTTP requests/responses?

Use the corresponding matcher in the `match` package. This allows you to be as specific or as lax as you like on your checks, and allows you to add JSON matchers on
the HTTP body.

#### I want to run a bunch of tests in parallel, how do I do this?

If you're familiar with Go testing then you already know how. Add `t.Parallel()` to all tests which you want to run in parallel. For a good example of this, see `registration_test.go` which does:
```go
// This will block until the 2 subtests have completed
t.Run("parallel", func(t *testing.T) {
    // run a subtest
    t.Run("POST {} returns a set of flows", func(t *testing.T) {
        t.Parallel()
        ...
    })
    // run another subtest
    t.Run("POST /register can create a user", func(t *testing.T) {
        t.Parallel()
        ...
    })
})
```

#### How should I do comments in the test?

Add long prose to the start of the function to outline what it is you're testing (and why if it is unclear). For example:
```go
// Test that a server can receive /keys requests:
// https://matrix.org/docs/spec/server_server/latest#get-matrix-key-v2-server-keyid
func TestInboundFederationKeys(t *testing.T) {
    ...
}
```

#### I think Complement is doing something weird, can I get more logs?

You can pass `COMPLEMENT_DEBUG=1` to add lots of debug logging. You can also do this via `os.Setenv("COMPLEMENT_DEBUG", "1")` before you make a deployment. This will add trace logging to the clients which logs full HTTP request/responses, amongst other debug info.

#### How do I set up a bunch of stuff before the tests, e.g before each?

There is no syntactically pleasing way to do this. Create a separate function which returns a function. See https://stackoverflow.com/questions/42310088/setup-and-teardown-for-each-test-using-std-testing-package?rq=1

#### How do I log messages in tests?

Standard Go testing here, use `t.Logf(...)` which will be logged only if the test fails or if `-v` is set. Note that you will not need to log HTTP requests performed using one of the built in deployment clients as they are already wrapped in loggers. For full HTTP logs, use `COMPLEMENT_DEBUG=1`.

#### How do I skip a test?

Use one of `t.Skipf(...)` or `t.SkipNow()`.

#### Why do we use `t.Errorf` sometimes and `t.Fatalf` other times?

Error will fail the test but continue execution, where Fatal will fail the test and quit. Use Fatal when continuing to run the test will result in programming errors (e.g nil exceptions).

#### Why do I get the error "Error response from daemon: Conflict. The container name "/complement_rooms_state_alice.hs1_1" is already in use by container "c2d1d90c6cff7b7de2678b56c702bd1ff76ca72b930e8f2ca32eef3f2514ff3b". You have to remove (or rename) that container to be able to reuse that name."?

The Docker daemon has a lag time between removing containers and them actually being removed. This means you cannot remove a container called 'foo' and immediately recreate it as 'foo'. To get around this, you need to use a different name. This probably means the namespace you have given the deployment is used by another test. Try changing it to something else e.g `must.Deploy(t, "rooms_state_2", b.BlueprintAlice.Name)`

#### How do I run tests inside my IDE?

For VSCode, add to `settings.json`:
```
"go.testEnvVars": {
    "COMPLEMENT_BASE_IMAGE": "complement-dendrite:latest"
}
```
