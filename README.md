<p align="right">
<a href="https://autorelease.general.dmz.palantir.tech/palantir/godel-mod-plugin"><img src="https://img.shields.io/badge/Perform%20an-Autorelease-success.svg" alt="Autorelease"></a>
</p>

mod-plugin
==========
`mod-plugin` is a [godel](https://github.com/palantir/godel) plugin that helps to standardize and verify the Go module
state for a project.

The task runs `go mod tidy` to standardize all of the module dependencies for a project. If the `GOFLAGS` environment
variable contains the value `-mod=vendor`, then this task will run `go mod vendor` after running `go mod tidy` to ensure
that the `vendor` directory state reflects the latest state.

The task also provides a "verify" mode that, when run, will exit with a non-0 exit code if running the core task causes
the checksum of the `go.mod`, `go.sum` or `vendor` paths to change. However, note that running in "verify" mode will
still modify local state. The behavior of verify mode will be improved once better first-class support for this
operation is provided by Go (see https://github.com/golang/go/issues/27005).

Tasks
-----
* `mod`: runs `go mod tidy` for the project. If `-mod=vendor` is specified in the `GOFLAGS` environment variable, then
  `go mod vendor` is performed after `go mod tidy`. 

Verify
------
When run as part of the `verify` task, if `apply=true`, then the `mod` task is run. If `apply=false`, the `mod` task is
run and the verification is considered to have failed if the checksums of `go.mod`, `go.sum` or `vendor` is changed by
the operation (note that, even if `apply=false`, the changes are applied).  
