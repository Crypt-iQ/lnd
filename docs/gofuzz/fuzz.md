# Fuzzing LND #

The `fuzz` package is organized into subpackages which are named after the `lnd` package they test. Each subpackage has its own set of fuzz targets.

### Setup and Installation ###
This section will cover setup and installation of `go-fuzz`.

* First, we must get `go-fuzz`.
* Note: If you already have `go-fuzz` installed and want to update, you may need to delete the `go-fuzz` and `go-fuzz-build` binaries and proceed normally.
```
$ go get github.com/dvyukov/go-fuzz/go-fuzz
$ go get github.com/dvyukov/go-fuzz/go-fuzz-build
```
* To build a message's fuzzing binary, you MUST NOT be in the `lnd` directory or any subdirectories. This is because of the way `go-fuzz` deals with go modules. The following command builds the channelupdate
message binary.
```
$ go-fuzz-build github.com/lightningnetwork/lnd/fuzz/lnwire/channelupdate
```

* This will create a file named `channelupdate-fuzz.zip`. It is recommended to move this to the `lnd` directory.

* Now, run `go-fuzz` from the `lnd` directory with `workdir` set as below!
```
$ go-fuzz -bin=<.zip archive here> -workdir=fuzz/lnwire/<message name here>
```

`go-fuzz` will print out log lines every couple of seconds. Example output:
```
2017/09/19 17:44:23 slaves: 8, corpus: 23 (3s ago), crashers: 1, restarts: 1/748, execs: 400690 (16694/sec), cover: 394, uptime: 24s
```
Corpus is the number of items in the corpus. `go-fuzz` may add valid inputs to
the corpus in an attempt to gain more coverage. Crashers is the number of inputs
resulting in a crash. The inputs, and their outputs are logged in:
`<folder name here>/crashers`. `go-fuzz` also creates a `suppressions` directory
of stacktraces to ignore so that it doesn't create duplicate stacktraces.
Cover is a number representing coverage of the program being fuzzed.

### Fuzzing Log ###
https://github.com/lightningnetwork/lnd/pull/310
https://github.com/lightningnetwork/lnd/pull/312
https://github.com/lightningnetwork/lnd/pull/1900

### Corpus ###
Fuzzing works best with a corpus that is of minimal size while achieving the maximum coverage. The corpus may seem large for each fuzzing target and non-optimal, but this is fine since `go-fuzz` automatically minimizes the corpus in-memory before fuzzing.

### Test Harness ###
If you take a look at the test harnesses that are used, you will see that they consist of one function: 
```
func Fuzz(data []byte) int
```
If:

- `-1` is returned, the fuzzing input is ignored
- `0` is returned, `go-fuzz` will add the input to the corpus and deprioritize it in future mutations.
- `1` is returned, `go-fuzz` will add the input to the corpus and prioritize it in future mutations.

### Conclusion ###
Citizens,
do your part and `go-fuzz` `lnd` today!
