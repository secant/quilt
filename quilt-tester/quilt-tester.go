package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"

	"github.com/NetSys/quilt/db"
	"github.com/NetSys/quilt/stitch"
)

const (
	testRoot           = "/tests"
	quiltPath          = "/.quilt"
	infrastructureSpec = quiltPath + "/github.com/NetSys/quilt/quilt-tester/" +
		"config/infrastructure.spec"
	slackEndpoint = "https://hooks.slack.com/services/T04Q3TL41/B0M25TWP5/" +
		"soKJeP5HbWcjkUJzEHh7ylYm"
	testerImport = "github.com/NetSys/quilt"
	webRoot      = "/var/www/quilt-tester"
)

// The global logger for this CI run.
var log logger

func main() {
	myIP := os.Getenv("MY_IP")
	if myIP == "" {
		logrus.Error("IP of tester machine unknown.")
		os.Exit(1)
	}

	var err error
	if log, err = newLogger(myIP); err != nil {
		logrus.WithError(err).Error("Failed to create logger.")
		os.Exit(1)
	}

	tester, err := newTester(testRoot, myIP)
	if err != nil {
		logrus.WithError(err).Error("Failed to create tester instance.")
		os.Exit(1)
	}

	if err := tester.run(); err != nil {
		logrus.WithError(err).Error("Test execution failed.")
		os.Exit(1)
	}
}

type tester struct {
	testSuites  []*testSuite
	initialized bool
	ip          string
}

func newTester(testRoot, myIP string) (tester, error) {
	t := tester{
		ip: myIP,
	}

	err := t.generateTestSuites(testRoot)
	if err != nil {
		return tester{}, err
	}

	return t, nil
}

func (t *tester) generateTestSuites(testRoot string) error {
	namespace := t.namespace()
	l := log.testerLogger

	// First, we need to ls the testRoot, and find all of the folders. Then we can
	// generate a testSuite for each folder.
	testSuiteFolders, err := filepath.Glob(filepath.Join(testRoot, "*"))
	if err != nil {
		l.infoln("Could not access test suite folders")
		l.errorln(err.Error())
		return err
	}

	for _, testSuiteFolder := range testSuiteFolders {
		files, err := ioutil.ReadDir(testSuiteFolder)
		if err != nil {
			l.infoln(fmt.Sprintf(
				"Error reading test suite %s", testSuiteFolder))
			l.errorln(err.Error())
			return err
		}

		var spec string
		var tests []string
		for _, file := range files {
			path := filepath.Join(testSuiteFolder, file.Name())
			switch {
			case strings.HasSuffix(file.Name(), ".spec"):
				spec = path
				if err := updateNamespace(spec, namespace); err != nil {
					l.infoln(fmt.Sprintf(
						"Error updating namespace for %s.", spec))
					l.errorln(err.Error())
					return err
				}
			// If the file is executable by everyone, and is not a directory.
			case (file.Mode()&1 != 0) && !file.IsDir():
				tests = append(tests, path)
			}
		}
		newSuite := testSuite{
			name:  filepath.Base(testSuiteFolder),
			spec:  spec,
			tests: tests,
		}
		t.testSuites = append(t.testSuites, &newSuite)
	}

	return nil
}

func (t tester) run() error {
	defer func() {
		cleanupMachines(t.namespace())
	}()

	if err := t.setup(); err != nil {
		log.testerLogger.errorln("Unable to setup the tests, bailing.")
		t.slack(false)
		return err
	}

	err := t.runTestSuites()
	t.slack(true)
	return err
}

func (t *tester) setup() error {
	namespace := t.namespace()
	l := log.testerLogger

	l.infoln("Starting the Quilt daemon.")
	go runQuiltDaemon()

	// Get our specs
	os.Setenv(stitch.QuiltPathKey, quiltPath)
	l.infoln(fmt.Sprintf("Downloading %s into %s", testerImport, quiltPath))
	_, _, err := downloadSpecs(testerImport)
	if err != nil {
		l.infoln(fmt.Sprintf("Could not download %s", testerImport))
		l.errorln(err.Error())
		return err
	}

	// Do a preliminary quilt stop.
	l.infoln(fmt.Sprintf("Preliminary `quilt stop %s`", namespace))
	_, _, err = stop(namespace)
	if err != nil {
		l.infoln(fmt.Sprintf("Error stopping: %s", err.Error()))
		return err
	}

	// Setup infrastructure.
	l.infoln("Booting the machines the test suites will run on, and waiting " +
		"for them to connect back.")
	l.infoln("Begin " + infrastructureSpec)
	if err := updateNamespace(infrastructureSpec, namespace); err != nil {
		l.infoln(fmt.Sprintf("Error updating namespace for %s.",
			infrastructureSpec))
		l.errorln(err.Error())
		return err
	}
	contents, _ := fileContents(infrastructureSpec)
	l.println(contents)
	l.infoln("End " + infrastructureSpec)

	_, _, err = runSpecUntilConnected(infrastructureSpec)
	if err != nil {
		l.infoln("Failed to setup infrastructure")
		l.errorln(err.Error())
		return err
	}

	l.infoln("Booted Quilt")
	l.infoln("Machines")
	machines, _ := queryMachines()
	l.println(fmt.Sprintf("%v", machines))

	return nil
}

func (t tester) runTestSuites() error {
	l := log.testerLogger

	machines, err := queryMachines()
	if err != nil {
		l.infoln("Unable to query test machines. Can't conduct tests.")
		return err
	}

	l.infoln("Wait 5 minutes for containers to start up")
	time.Sleep(5 * time.Minute)

	for _, suite := range t.testSuites {
		if e := suite.run(machines); e != nil && err == nil {
			err = e
		}
	}
	return err
}

func (t tester) namespace() string {
	sanitizedIP := strings.Replace(t.ip, ".", "-", -1)
	return fmt.Sprintf("tester-%s", sanitizedIP)
}

func toPost(failed bool, pretext string, text string) slackPost {
	iconemoji := ":confetti_ball:"
	color := "#009900" // Green
	if failed {
		iconemoji = ":oncoming_police_car:"
		color = "#D00000" // Red
	}

	return slackPost{
		Channel:   os.Getenv("SLACK_CHANNEL"),
		Color:     color,
		Pretext:   pretext,
		Username:  "quilt-bot",
		Iconemoji: iconemoji,
		Fields: []message{
			{
				Title: "Continuous Integration",
				Short: false,
				Value: text,
			},
		},
	}
}

func (t tester) slack(initialized bool) {
	log.testerLogger.infoln("Posting to slack.")

	var suitesPassed []string
	var suitesFailed []string
	for _, suite := range t.testSuites {
		if suite.failed != 0 {
			suitesFailed = append(suitesFailed, suite.name)
		} else {
			suitesPassed = append(suitesPassed, suite.name)
		}
	}

	var failed bool
	var pretext, text string
	if !initialized {
		failed = true
		text = "Didn't run tests"
		pretext = fmt.Sprintf("<!channel> Initialization <%s|failed>.",
			log.url())
	} else {
		// The tests passed.
		failed = false
		pretext = fmt.Sprintf("All tests <%s|passed>!", log.url())
		text = fmt.Sprintf("Test Suites Passed: %s",
			strings.Join(suitesPassed, ", "))

		// Some tests failed.
		if len(suitesFailed) > 0 {
			failed = true
			text += fmt.Sprintf("\nTest Suites Failed: %s",
				strings.Join(suitesFailed, ", "))
			pretext = fmt.Sprintf("<!channel> Some tests <%s|failed>",
				log.url())
		}
	}

	err := slack(slackEndpoint, toPost(failed, pretext, text))
	if err != nil {
		l := log.testerLogger
		l.infoln("Error posting to Slack.")
		l.errorln(err.Error())
	}
}

type testSuite struct {
	name   string
	spec   string
	tests  []string
	passed int
	failed int
}

func (ts *testSuite) run(machines []db.Machine) error {
	l := log.testerLogger

	l.infoln(fmt.Sprintf("Test Suite: %s", ts.name))
	l.infoln("Start " + ts.name + ".spec")
	contents, _ := fileContents(ts.spec)
	l.println(contents)
	l.infoln("End " + ts.name + ".spec")

	runSpec(ts.spec)

	// Wait for the containers to start
	l.infoln("Waiting 5 minutes for containers to start up")
	time.Sleep(5 * time.Minute)
	l.infoln("Starting Tests")
	var err error
	for _, machine := range machines {
		l.println("\n" + machine.PublicIP)
		for _, test := range ts.tests {
			if strings.Contains(test, "monly") && machine.Role != "Master" {
				continue
			}

			l.println(".. " + filepath.Base(test))
			passed, e := runTest(test, machine)
			if passed {
				l.println(".... Passed")
				ts.passed++
			} else {
				l.println(".... Failed")
				ts.failed++
			}

			if e == nil {
				err = e
			}
		}
		l.println("")
	}

	l.infoln("Finished Tests")
	l.infoln(fmt.Sprintf("Finished Test Suite: %s", ts.name))

	return err
}

func runTest(testPath string, m db.Machine) (bool, error) {
	_, testName := filepath.Split(testPath)

	// Run the test on the remote machine.
	err := scp(m.PublicIP, testPath, testName)
	if err != nil {
		return false, err
	}
	sshCmd := sshGen(m.PublicIP, exec.Command(fmt.Sprintf("./%s", testName)))
	output, err := sshCmd.CombinedOutput()
	if err != nil {
		return false, err
	}

	testPassed := true
	if !strings.Contains(string(output), "PASSED") {
		testPassed = false
	}

	l := log.testLogger(testPassed, testName, m.PublicIP)
	if !testPassed {
		l.infoln("Failed!")
	}

	if contents, err := fileContents(testPath + ".go"); err == nil {
		l.infoln("Begin test source")
		l.println(contents)
		l.infoln("End test source")
	} else {
		l.infoln(fmt.Sprintf("Could not read test source for %s", testName))
		l.errorln(err.Error())
	}

	l.infoln("Begin test output")
	l.println(string(output))
	l.infoln("End test output")

	return testPassed, nil
}
