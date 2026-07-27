<p align="right">
<a href="https://autorelease.general.dmz.palantir.tech/palantir/godel-mod-plugin"><img src="https://img.shields.io/badge/Perform%20an-Autorelease-success.svg" alt="Autorelease"></a>
</p>

godel-mod-plugin
================
`godel-mod-plugin` is a [godel](https://github.com/palantir/godel) plugin that helps to standardize and verify the Go
module state for a project.

The task runs `go mod tidy` to standardize all of the module dependencies for a project. If the `GOFLAGS` environment
variable contains the value `-mod=vendor`, then this task will run `go mod vendor` after running `go mod tidy` to ensure
that the `vendor` directory state reflects the latest state.

Because `go mod vendor` resets the entire `vendor` directory, the task vendors into a temporary directory and writes
only the files that actually differ back into the project. Running the task when the project is already up-to-date
therefore leaves the content and modification times of the `vendor` directory alone rather than rewriting every file in
it. File modes are brought in line without content being rewritten.

The task also provides a "verify" mode that, when run, will exit with a non-0 exit code if the `go.mod`, `go.sum` or
`vendor` state is not up-to-date. Verify mode does not modify the project: it runs `go mod tidy -diff`, which reports
the changes tidying would make without applying them, and compares the `vendor` directory against the temporary
directory without writing to it. In both cases the differences that caused the failure are printed.

Tasks
-----
* `mod`: runs `go mod tidy` for the project. If `-mod=vendor` is specified in the `GOFLAGS` environment variable, then
  `go mod vendor` is performed after `go mod tidy`. 

Verify
------
When run as part of the `verify` task, if `apply=true`, then the `mod` task is run. If `apply=false`, the verification
is considered to have failed if `go.mod`, `go.sum` or `vendor` is not up-to-date, and the project is left unmodified.  
