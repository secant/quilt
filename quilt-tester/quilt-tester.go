package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"

	"github.com/NetSys/quilt/api"
	"github.com/NetSys/quilt/db"
)

var configSpec, configSpecName, namespace, testRoot, specDir, url string
var l log
var suitesPassed, suitesFailed []string
var machines []db.Machine

type log struct {
	passedDir   string
	failedDir   string
	logDir      string
	currTest    string
	container   string
	quiltTester string
}

type format func(string) string

func (l *log) logInfo(msg string, fn format) {
	if err := writeTo(l.quiltTester, fn(msg)); err != nil {
		panic(err)
	}
}

func (l *log) logTest(msg string, fn format) {
	if err := writeTo(l.currTest, fn(msg)); err != nil {
		panic(err)
	}
}

func (l *log) logContainer(msg string, fn format) {
	if err := writeTo(l.container, fn(msg)); err != nil {
		panic(err)
	}
}

func infoMsg(msg string) string {
	timestamp := time.Now().Format("[15:04:05] ")
	return "\n" + timestamp + "=== " + msg + " ===\n"
}

func errMsg(err string) string {
	return "\n=== Error Text ===\n" + err + "\n"
}

func verbMsg(msg string) string {
	return "\n" + msg + "\n"
}

func writeTo(file string, message string) error {
	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		fmt.Println("Could not open", file)
		return err
	}

	defer f.Close()
	_, err = f.WriteString(message)
	return err
}

func fileContents(file string) (string, error) {
	contents, err := ioutil.ReadFile(file)
	if err != nil {
		return "", err
	}
	return string(contents), nil
}

type testSuite struct {
	name   string
	spec   string
	tests  []string
	passed int
	failed int
}

func (ts *testSuite) run() error {
	l.logInfo(fmt.Sprintf("Test Suite: %s", ts.name), infoMsg)
	l.logInfo("Start "+ts.name+".spec", infoMsg)
	contents, err := fileContents(ts.spec)
	if err != nil {
		return err
	}

	l.logInfo(contents, verbMsg)
	l.logInfo("End "+ts.name+".spec", infoMsg)
	runSpec(ts.spec)

	// Wait for the containers to start
	l.logInfo("Waiting 5 minutes for containers to start up", infoMsg)
	time.Sleep(300 * time.Second)
	l.logInfo("Starting Tests", infoMsg)
	for _, machine := range machines {
		for _, test := range ts.tests {
			if strings.Contains(test, "monly") &&
				machine.Role != "Master" {
				continue
			}
			if err := runTest(test, machine); err != nil {
				ts.failed++
			} else {
				ts.passed++
			}
		}
	}

	if ts.failed > 0 {
		suitesFailed = append(suitesFailed, ts.name)
	} else {
		suitesPassed = append(suitesPassed, ts.name)
	}

	l.logInfo("Finished Tests", infoMsg)
	l.logInfo(fmt.Sprintf("Finished Test Suite: %s", ts.name), infoMsg)

	return nil
}

func runTest(test string, m db.Machine) error {
	dir, file := filepath.Split(test)
	contents, err := fileContents(filepath.Join(dir, "src", file, file) + ".go")
	if err != nil {
		l.logInfo(fmt.Sprintf("Could not read test %s", test), infoMsg)
		l.logInfo(err.Error(), errMsg)
		return err
	}

	testLog := file + "-" + m.PublicIP + ".txt"
	l.currTest = filepath.Join(l.passedDir, testLog)

	scp(m.PublicIP, test, file)
	sshCmd := sshGen(m.PublicIP, exec.Command(fmt.Sprintf("./%s", file)))
	output, err := sshCmd.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "PASSED") {
		l.currTest = filepath.Join(l.failedDir, testLog)
		l.logTest("Failed!", infoMsg)
		if err == nil {
			err = errors.New("failed test")
		}
	}

	l.logTest("Begin test source", infoMsg)
	l.logTest(contents, verbMsg)
	l.logTest("End test source", infoMsg)
	l.logTest("Begin test output", infoMsg)
	l.logTest(string(output), verbMsg)
	fmt.Println(string(output))
	l.logTest("End test output", infoMsg)
	return err
}

func runSpec(spec string) (string, error) {
	cmd := exec.Command("/quiltctl", "run", "-stitch", spec)
	out, err := execCmd(cmd, execOptions{
		logLineTitle: "RUN",
	})
	return out, err
}

func setupInfrastructure() (string, error) {
	cmd := exec.Command("/quiltctl", "run", "-stitch", configSpecName)
	out, err := execCmd(cmd, execOptions{
		logLineTitle: "INFRA",
	})
	if err != nil {
		return out, err
	}

	allConnected := func() bool {
		machines, err := queryMachines()
		if err != nil {
			return false
		}

		for _, m := range machines {
			if !m.Connected {
				return false
			}
		}

		return true
	}
	return out, waitFor(allConnected, 500)
}

// waitFor waits until `pred` is satisfied, or `timeout` seconds have passed.
func waitFor(pred func() bool, timeout int) error {
	for range time.Tick(1 * time.Second) {
		select {
		case <-time.After(time.Duration(timeout) * time.Second):
			return errors.New("waitFor timed out")
		default:
			if pred() {
				return nil
			}
		}
	}
	return nil
}

type execOptions struct {
	logLineTitle string
	stopChan     chan struct{}
}

// execCmd executes the given command, and returns the Stderr output.
// If `logLineTitle` is non-empty, then each line of the command is logged to container.log.
// If `stopChan` is closed, then the command is immediately killed.
func execCmd(cmd *exec.Cmd, opts execOptions) (string, error) {
	if opts.logLineTitle != "" {
		l.logContainer(fmt.Sprintf("%s: Starting command: %v", opts.logLineTitle, cmd.Args), infoMsg)
	}

	// Save the command stderr output to `output` while logging it.
	pipe, err := cmd.StderrPipe()
	if err != nil {
		panic(err)
	}

	output := ""
	go func(reader io.Reader) {
		outScanner := bufio.NewScanner(pipe)
		for outScanner.Scan() {
			outStr := outScanner.Text()
			output += outStr
			if opts.logLineTitle != "" {
				// Remove the newline if there is one because logContainer appends one automatically.
				logStr := strings.TrimSuffix(outStr, "\n")
				l.logContainer(opts.logLineTitle+": "+logStr, verbMsg)
			}
		}
	}(pipe)

	if err := cmd.Start(); err != nil {
		return "", err
	}

	cmdFinished := make(chan error, 1)
	go func() {
		cmdFinished <- cmd.Wait()
	}()

	select {
	case <-cmdFinished:
		l.logContainer(fmt.Sprintf("%s: Completed command: %v", opts.logLineTitle, cmd.Args), infoMsg)
		return output, nil
	case <-opts.stopChan:
		return output, cmd.Process.Kill()
	}
}

func quiltDaemon(stop chan struct{}) {
	l.logInfo("Starting the Quilt daemon.", infoMsg)
	cmd := exec.Command("/quilt")
	execCmd(cmd, execOptions{
		stopChan:     stop,
		logLineTitle: "QUILT",
	})
}

func generateTestSuites(testDir string) []testSuite {
	var suites []testSuite
	// First, we need to ls the testDir, and find all of the folders. Then we can
	// generate a testSuite for each folder.
	testFiles, err := filepath.Glob(filepath.Join(testDir, "*"))
	if err != nil {
		l.logInfo("Could not access test suite folders", infoMsg)
		l.logInfo(err.Error(), errMsg)
		return []testSuite{}
	}

	for _, testPath := range testFiles {
		info, err := os.Stat(testPath)
		if err != nil {
			l.logInfo(fmt.Sprintf("Could not access path %s", testPath),
				infoMsg)
			l.logInfo(err.Error(), errMsg)
			return suites
		}

		if info.IsDir() {
			specs, err := filepath.Glob(filepath.Join(testPath, "*.spec"))
			if err != nil {
				l.logInfo(fmt.Sprintf("Could not get spec file for %s",
					testPath), infoMsg)
				l.logInfo(err.Error(), errMsg)
				continue
			}
			spec := specs[0]
			allFiles, err := filepath.Glob(filepath.Join(testPath, "*"))
			if err != nil {
				l.logInfo(fmt.Sprintf("Could not access %s test files",
					testPath), infoMsg)
				l.logInfo(err.Error(), errMsg)
				continue
			}
			var tests []string
			for _, file := range allFiles {
				info, _ := os.Stat(file)
				if filepath.Ext(file) == "" && !info.IsDir() {
					tests = append(tests, file)
				}
			}
			newSuite := testSuite{
				name:  filepath.Base(testPath),
				spec:  spec,
				tests: tests,
			}
			suites = append(suites, newSuite)
		}
	}
	return suites
}

func runTestSuites(suites []testSuite) error {
	// Do a preliminary quilt stop.
	l.logInfo(fmt.Sprintf("Preliminary `quilt stop %s`", namespace), infoMsg)
	_, err := stop()
	if err != nil {
		l.logInfo(fmt.Sprintf("Error stopping: %s", err.Error()), verbMsg)
		return err
	}

	l.logInfo("Begin "+configSpec, infoMsg)
	contents, err := fileContents(configSpec)
	if err != nil {
		return err
	}

	l.logInfo(contents, verbMsg)
	l.logInfo("End "+configSpec, infoMsg)
	_, err = setupInfrastructure()
	if err != nil {
		l.logInfo(fmt.Sprintf("Failed to load spec %s", configSpec), infoMsg)
		l.logInfo(err.Error(), errMsg)
		cleanup()
		return err
	}

	populateMachines()
	l.logInfo("Booted Quilt", infoMsg)
	l.logInfo("Machines", infoMsg)
	l.logInfo(fmt.Sprintf("%v", machines), verbMsg)
	l.logInfo("Wait 5 minutes for containers to start up", infoMsg)
	time.Sleep(300 * time.Second)
	for _, suite := range suites {
		if err := suite.run(); err != nil {
			l.logInfo(fmt.Sprintf("Error running test suite %s",
				suite.name), infoMsg)
			l.logInfo(err.Error(), errMsg)
		}
	}
	cleanup()
	slack()
	return nil
}

func queryMachines() ([]db.Machine, error) {
	c, err := api.NewClient(api.DefaultSocket)
	if err != nil {
		return []db.Machine{}, err
	}

	return c.QueryMachines()
}

func populateMachines() {
	var err error
	machines, err = queryMachines()
	if err != nil {
		l.logInfo(fmt.Sprintf("Unable to query Quilt machines: %s", err.Error()), verbMsg)
		return
	}
}

// Begin Cleanup Functions
func cleanup() {
	l.logInfo("Cleaning up first with `quilt stop`.", infoMsg)
	if _, err := stop(); err != nil {
		l.logInfo("`quilt stop` errored.", infoMsg)
		l.logInfo(err.Error(), errMsg)
		l.logInfo("Now attempting to use killAWS.", infoMsg)
		if err := killAWS(); err != nil {
			l.logInfo("killAWS errored.", infoMsg)
			l.logInfo(err.Error(), errMsg)
		}
	}
	l.logInfo("Done cleaning up.", infoMsg)
}

func stop() (string, error) {
	cmd := exec.Command("/quiltctl", "stop", "-namespace", namespace)

	out, err := execCmd(cmd, execOptions{
		logLineTitle: "STOP",
	})
	if err != nil {
		return out, err
	}

	stopped := func() bool {
		return len(getAWSInstances()) == 0
	}
	return out, waitFor(stopped, 120)
}

func getAWSInstances() []*string {
	// Find all of the instances under the namespace
	svc := ec2.New(session.New(), &aws.Config{Region: aws.String("us-west-1")})
	params := &ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{Name: aws.String("network-interface.group-name"),
				Values: []*string{aws.String(namespace)}},
		},
	}
	resp, err := svc.DescribeInstances(params)
	if err != nil {
		l.logInfo(fmt.Sprintf("Unable to get AWS instances: %s", err.Error()), verbMsg)
		return []*string{}
	}

	if len(resp.Reservations) == 0 {
		return []*string{}
	}

	// Build the list of InstanceIds
	ids := []*string{}
	for _, res := range resp.Reservations {
		for _, inst := range res.Instances {
			ids = append(ids, inst.InstanceId)
		}
	}
	return ids
}

func killAWS() error {
	ids := getAWSInstances()

	toDelete := &ec2.TerminateInstancesInput{
		InstanceIds: ids,
	}

	svc := ec2.New(session.New(), &aws.Config{Region: aws.String("us-west-1")})
	req, _ := svc.TerminateInstancesRequest(toDelete)
	return req.Send()
}

// End Cleanup Functions

// Begin Spec Functions
func generateNamespace() {
	ip := os.Getenv("MY_IP")
	ip = strings.Replace(ip, ".", "-", -1)
	namespace = fmt.Sprintf("tester-%s", ip)
}

// This function updates the spec with a correct namespace and sshkey.
func changeSpec(specfile string) error {
	defNamespace := fmt.Sprintf(`(define Namespace "%s")`, namespace) + "\n"
	input, err := ioutil.ReadFile(specfile)
	if err != nil {
		return err
	}

	if !strings.Contains(string(input), defNamespace) {
		return writeTo(specfile, "\n"+defNamespace)
	}
	return nil
}

// End Spec Functions

// CLI Wrapper Functions
func sshGen(host string, cmd *exec.Cmd) *exec.Cmd {
	script := "ssh"
	args := []string{"-o", "UserKnownHostsFile=/dev/null", "-o",
		"StrictHostKeyChecking=no", fmt.Sprintf("quilt@%s", host)}
	args = append(args, cmd.Args...)
	sshCmd := exec.Command(script, args...)
	return sshCmd
}

func scp(host string, source string, target string) error {
	cmd := exec.Command("scp", "-o", "UserKnownHostsFile=/dev/null", "-o",
		"StrictHostKeyChecking=no", source,
		fmt.Sprintf("quilt@%s:%s", host, target))
	return cmd.Run()
}

func slack() {
	type field struct {
		Title string `json:"title"`
		Short bool   `json:"short"`
		Value string `json:"value"`
	}

	type payload struct {
		Channel   string  `json:"channel"`
		Color     string  `json:"color"`
		Fields    []field `json:"fields"`
		Pretext   string  `json:"pretext"`
		Username  string  `json:"username"`
		Iconemoji string  `json:"icon_emoji"`
	}

	iconemoji := ":confetti_ball:"
	pretext := fmt.Sprintf("All tests <%s|passed>!", url)
	color := "#009900"
	value := fmt.Sprintf("Test Suites Passed: %s", strings.Join(suitesPassed, ", "))
	if len(suitesFailed) > 0 {
		value = value + fmt.Sprintf("\nTest Suites Failed: %s",
			strings.Join(suitesFailed, ", "))
		iconemoji = ":oncoming_police_car:"
		pretext = fmt.Sprintf("<!channel> Some tests <%s|failed>", url)
		color = "#D00000" // Red
	}
	f := field{
		Title: "Continuous Integration",
		Short: false,
		Value: value}

	p := payload{
		Channel:   os.Getenv("SLACK_CHANNEL"),
		Color:     color, // Green
		Pretext:   pretext,
		Username:  "quilt-bot",
		Iconemoji: iconemoji,
		Fields:    []field{f}}

	hookurl := "https://hooks.slack.com/services/T04Q3TL41/B0M25TWP5/soKJeP5HbWcjk" +
		"UJzEHh7ylYm"
	body, err := json.Marshal(p)
	if err != nil {
		panic(err)
	}

	resp, err := http.Post(hookurl, "application/json", bytes.NewReader(body))
	if err != nil {
		panic(err)
	}

	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t, _ := ioutil.ReadAll(resp.Body)
		l.logInfo("Error posting to Slack.", infoMsg)
		l.logInfo(string(t), errMsg)
	}
}

func downloadSpecs(importPath string) {
	quiltPath := os.Getenv("QUILT_PATH")
	l.logInfo(fmt.Sprintf("Downloading %s into %s", importPath, quiltPath), infoMsg)
	cmd := exec.Command("/quiltctl", "get", "-importPath", importPath)
	_, err := execCmd(cmd, execOptions{
		logLineTitle: "GET",
	})
	if err != nil {
		l.logInfo(fmt.Sprintf("Could not download %s", importPath), infoMsg)
		l.logInfo(err.Error(), errMsg)
		os.Exit(1)
	}
}

func main() {
	configSpecName = "config/infrastructure.spec"
	configSpec = "/.quilt/config/infrastructure.spec"
	testRoot, err := filepath.Abs("/")
	if err != nil {
		panic(err)
	}

	testDir := filepath.Join(testRoot, "tests")
	specDir = filepath.Join(testRoot, "specs")
	webRoot, err := filepath.Abs("/var/www/quilt-tester")
	if err != nil {
		panic(err)
	}

	// Add the web directory for logs
	webDir := filepath.Join(webRoot, time.Now().Format("02-01-2006_15h04m05s"))
	url = fmt.Sprintf("http://%s/%s", os.Getenv("MY_IP"), filepath.Base(webDir))
	logDir := filepath.Join(webDir, "log")
	l = log{
		logDir:      logDir,
		passedDir:   filepath.Join(webDir, "passed"),
		failedDir:   filepath.Join(webDir, "failed"),
		quiltTester: filepath.Join(logDir, "quilt-tester.log"),
		container:   filepath.Join(logDir, "container.log"),
	}

	if err := os.MkdirAll(l.logDir, 0755); err != nil {
		panic(err)
	}

	if err := os.Mkdir(l.passedDir, 0755); err != nil {
		panic(err)
	}

	if err := os.Mkdir(l.failedDir, 0755); err != nil {
		panic(err)
	}

	if err := os.Remove(filepath.Join(webRoot, "latest")); err != nil {
		panic(err)
	}

	if err := os.Symlink(webDir, filepath.Join(webRoot, "latest")); err != nil {
		panic(err)
	}

	daemon := make(chan struct{})
	go quiltDaemon(daemon)

	// Get our specs
	os.Setenv("QUILT_PATH", "/.quilt")
	downloadSpecs("github.com/NetSys/quilt")
	generateNamespace()
	suites := generateTestSuites(testDir)
	runTestSuites(suites)

	close(daemon)
}
