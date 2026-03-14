package fail

fail

/*
This is a non-compiling file that has been added to explicitly ensure that CI fails.
It also contains the command that caused the failure and its output.
Remove this file if debugging locally.

./godelw verify failed after updating godel plugins and assets

Command that caused error:
./godelw exec -- go fix ./...

Output:
# github.com/palantir/godel-mod-plugin/gomod
fix: applied 2 of 3 fixes; 1 file updated. (Re-run the command to apply more.)
Error: exit status 1

*/
