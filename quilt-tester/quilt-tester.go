package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
)

var configSpec, configSpecName, namespace, testRoot, specDir, url string
var l log
var suitesPassed, suitesFailed []string
var machines []map[string]string

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
	_, cmd, _ := runSpec(ts.spec)

	// Wait for the containers to start
	l.logInfo("Waiting 5 minutes for containers to start up", infoMsg)
	time.Sleep(300 * time.Second)
	l.logInfo("Starting Tests", infoMsg)
	for _, machine := range machines {
		for _, test := range ts.tests {
			if strings.Contains(test, "monly") &&
				machine["role"] != "Master" {
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

	// Kill the current quilt process
	if err := cmd.Process.Kill(); err != nil {
		panic(err)
	}
	return nil
}

func runTest(test string, m map[string]string) error {
	dir, file := filepath.Split(test)
	contents, err := fileContents(filepath.Join(dir, "src", file, file) + ".go")
	if err != nil {
		l.logInfo(fmt.Sprintf("Could not read test %s", test), infoMsg)
		l.logInfo(err.Error(), errMsg)
		return err
	}

	testLog := file + "-" + m["publicIP"] + ".txt"
	l.currTest = filepath.Join(l.passedDir, testLog)

	scp(m["publicIP"], test, file)
	sshCmd := sshGen(m["publicIP"], exec.Command(fmt.Sprintf("./%s", file)))
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

func runSpec(spec string) (string, *exec.Cmd, error) {
	cmd := exec.Command("/quilt", "run", spec)
	if spec == configSpecName {
		output, err := runAndWaitFor(cmd, "New connection.", 500)
		return output, cmd, err
	}
	l.logContainer(fmt.Sprintf("Running %v for 60 seconds", cmd.Args), infoMsg)
	cmd.Start()
	time.Sleep(60 * time.Second)
	return "", cmd, nil
}

func runAndWaitFor(cmd *exec.Cmd, trigger string, timeout int) (string, error) {
	l.logContainer(fmt.Sprintf("Running %v until %s", cmd.Args, trigger), infoMsg)
	output := ""
	pipe, err := cmd.StderrPipe()
	if err != nil {
		panic(err)
	}

	if err := cmd.Start(); err != nil {
		panic(err)
	}

	done := make(chan error, 1)
	timer := time.After(time.Duration(timeout) * time.Second)
	go func() {
		done <- cmd.Wait()
	}()

	info := make([]byte, 2048, 2048)
	infoStr := ""
	go func() {
		for range time.Tick(1 * time.Second) {
			select {
			case <-done:
				return
			default:
				pipe.Read(info)
				// Remove trailing null chracters
				infoStr = string(bytes.Trim(info, "\x00"))
				if len(infoStr) == 0 {
					continue
				}
				fmt.Println(infoStr)
				l.logContainer(infoStr, verbMsg)
				output += infoStr
				if strings.Contains(infoStr, trigger) {
					done <- nil
					return
				}
			}
		}
	}()

	select {
	case <-timer:
		done <- nil
		if err := cmd.Process.Kill(); err != nil {
			panic(err)
		}
		return output, fmt.Errorf("Quilt timed out while waiting for "+
			"%s. Output can be found in container.log.", trigger)

	case err := <-done:
		return output, err
	}
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
	stop()
	l.logInfo("Begin "+configSpec, infoMsg)
	contents, err := fileContents(configSpec)
	if err != nil {
		return err
	}

	l.logInfo(contents, verbMsg)
	l.logInfo("End "+configSpec, infoMsg)
	output, cmd, err := runSpec(configSpecName)
	if err != nil {
		l.logInfo(fmt.Sprintf("Failed to load spec %s", configSpec), infoMsg)
		l.logInfo(err.Error(), errMsg)
		cleanup()
		return err
	}

	populateMachines(output)
	l.logInfo("Booted Quilt", infoMsg)
	l.logInfo("Machines", infoMsg)
	l.logInfo(fmt.Sprintf("%v", machines), verbMsg)
	l.logInfo("Wait 5 minutes for containers to start up", infoMsg)
	time.Sleep(300 * time.Second)
	// Kill the current quilt process
	if err := cmd.Process.Kill(); err != nil {
		panic(err)
	}
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

func populateMachines(output string) {
	grabbed := false
	numM := 0
	numRe := regexp.MustCompile(`count=(\d)`)
	numM, _ = strconv.Atoi(numRe.FindStringSubmatch(output)[1])
	machineRe := regexp.MustCompile(`Machine-(\d+){Role=(.*), ` +
		"Provider=(.*), Region=(.*), Size=(.*), DiskSize=(.*), CloudID=(.*), " +
		"PublicIP=(.*), PrivateIP=(.*)}")
	bootsRe := regexp.MustCompile("Successfully booted machines.")
	boots := bootsRe.FindAllStringIndex(output, -1)
	output = output[boots[len(boots)-1][0]:] // Trim to the last successful boot
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "db.Machine") {
			grabbed = true
		}

		if grabbed && len(machines) >= numM {
			break
		}

		if grabbed && strings.Contains(line, "Machine-") {
			if !strings.Contains(line, "PublicIP") {
				grabbed = false
				continue
			}
			matches := machineRe.FindStringSubmatch(line)
			m := make(map[string]string)
			m["name"] = matches[1]
			m["role"] = matches[2]
			m["provider"] = matches[3]
			m["region"] = matches[4]
			m["cloudID"] = matches[5]
			m["size"] = matches[6]
			m["disksize"] = matches[7]
			m["publicIP"] = matches[8]
			m["privateIP"] = matches[9]
			machines = append(machines, m)
		}
	}
}

// Begin Cleanup Functions
func cleanup() {
	l.logInfo("Cleaning up first with `quilt stop`.", infoMsg)
	if err := stop(); err != nil {
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

func stop() error {
	cmd := exec.Command("/quilt", "stop", namespace)
	_, err := runAndWaitFor(cmd, "Successfully halted machines.", 120)
	if err == nil {
		if err := cmd.Process.Kill(); err != nil {
			panic(err)
		}
	}
	return err
}

func killAWS() error {
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
		return err
	}

	if len(resp.Reservations) == 0 {
		return nil
	}

	// Build the list of InstanceIds
	ids := []*string{}
	for _, res := range resp.Reservations {
		for _, inst := range res.Instances {
			ids = append(ids, inst.InstanceId)
		}
	}

	toDelete := &ec2.TerminateInstancesInput{
		InstanceIds: ids,
	}

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
	cmd := exec.Command("/quilt", "get", importPath)
	if err := cmd.Run(); err != nil {
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

	os.Setenv("PATH", os.Getenv("PATH")+":"+filepath.Join(testRoot, "bin"))
	fmt.Println("PATH:", os.Getenv("PATH"))

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

	// Get our specs
	os.Setenv("QUILT_PATH", "/.quilt")
	downloadSpecs("github.com/NetSys/quilt")
	generateNamespace()
	suites := generateTestSuites(testDir)
	runTestSuites(suites)
}
