# Quilt Continuous Integration

A lot of this code depends on the specific way Quilt logs things. For example,
if the output for the machine table were to change, we wouldn't be able to
get the IP addresses. We also watch the output for specific strings to trigger
our actions. The eventual official `quilt-ctl` tool will help in the future,
though.

`quilt-tester` pulls the latest Quilt code from master, builds it, runs it with
minions on AWS, and then ssh's into each of the servers and runs a test script.
If the exit code of the test script is non-zero, then `quilt-tester` interprets
this test as having failed.

## Setup
[setup](vagrant/setup) should take care of everything.

For local instances:
```
cd vagrant
vagrant up && vagrant ssh
sudo ./setup # do this inside the VM
```

Make sure that if you're testing a fork of the project, that `quilt-tester`
is included as a user.

On AWS, just scp the setup file over and do the same thing.

The script will ask you a couple questions to configure itself.

The IP address is used to generate links to test results.

The slack channel is used to determine where to post results. You can
use `@$USER` (e.g. @kklin) to get the results DMd.

The "aws credentials cron" is a cron job that updates the aws credentials file
every night. Only use this if you're on AWS, and IAM roles are properly setup
for the host.

The `Vagrantfile` tries to copy your aws credentials over automatically from
the host computer, so other than answering the initial questions, you shouldn't
have to setup anything else.

## Usage
You can trigger a new test run by sending a GET or POST request to
`http://$IP/cgi-bin/trigger_run`.

## Adding tests
Tests are written in Go and cross-compiled into an executable with the command

```bash
env GOOS=linux GOARCH=amd64 go build <TEST_NAME>.go
```

To add a test, place it in the [tests](tests) directory. Each folder represents
a *test suite*, which contains a spec (with the same name as the folder) and
tests to run on that specific spec. For example, [tests/spark](tests/spark) runs
`spark.spec` and checks that the Spark job approximated pi. Print `"PASSED"` if
a test passes, and `"FAILED"` if a test fails. If you would like output from
commands to be saved, make sure you print them (See the [os/exec](https://golang.org/pkg/os/exec/)
library for more. If you want to run the test on the master machine only, append `_monly` to the
name of your test file.

## Logging
When things break, you can take a look at the `quilt-tester` log. Each run
saves logs to `/var/www/quilt-tester/$RUN/logs`.

A quick way to watch the logs is
`tail -f "$(\ls -1dt /var/www/quilt-tester/*/ | head -n 1)/logs/quilt-tester.log"`

## Security
`~/.ssh` must contain a private key associated with a GitHub account for the
Quilt repo. The associated public key must be in the spec file. A default key
and spec is provided in the repo.

Additionally, aws credentials must be present in `~/.aws`

## Apache
Apache needs to be configured to serve up `$WEB_ROOT` as defined in [tester](bin/tester).
The `setup` script should handle all the details.

## Modifying the testing interval
By default, the tests are automatically run every hour. You can tweak
`quilt-tester`'s crontab (using `crontab -e` as `quilt-tester`) to change the
time interval.

## Updating the Tester
When you update `quilt-tester.go`, make sure to cross-compile it (just like you
would with a test file) and place it in `bin/`.

## XXX:
- Setup post-commit hooks on Github for updating the tests folder and testing
  new merges
- Package quilt-controller, quilt-minion, and quilt-tester as containers for
  easy deployment
- Make sure we clean up after each test
    - Kill the minions using `aws cli` if the controller hangs
    - Bail early if things fail in the script
    - Assume failure if the minions don't connect after a certain timeout
- Ping kklin on slack if the tester seems to be failing (e.g. if we're timing
  out)
- Differentiate tests for master and workers?
- Create summary of names of tests that failed
- Do a security audit
- Allow only one instance of `quilt-tester` to run per machine at a time
- Remove the `quilt-tester` ssh key
