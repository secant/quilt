package stitch

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/spf13/afero"

	"github.com/NetSys/quilt/util"
)

func TestMachine(t *testing.T) {
	t.Parallel()

	checkMachines(t, `deployment.deploy([new Machine({
		role: "Worker",
		provider: "Amazon",
		region: "us-west-2",
		size: "m4.large",
		cpu: new Range(2, 4),
		ram: new Range(4, 8),
		diskSize: 32,
		sshKeys: ["key1", "key2"]
	})])`,
		[]Machine{
			{
				Role:     "Worker",
				Provider: "Amazon",
				Region:   "us-west-2",
				Size:     "m4.large",
				CPU:      Range{2, 4},
				RAM:      Range{4, 8},
				DiskSize: 32,
				SSHKeys:  []string{"key1", "key2"},
			}})

	checkMachines(t, `var baseMachine = new Machine({provider: "Amazon"});
		deployment.deploy(baseMachine.asMaster().replicate(2));`,
		[]Machine{
			{
				Role:     "Master",
				Provider: "Amazon",
				SSHKeys:  []string{},
			},
			{
				Role:     "Master",
				Provider: "Amazon",
				SSHKeys:  []string{},
			},
		},
	)

	checkMachines(t, `var baseMachine = new Machine({provider: "Amazon"});
		var machines = baseMachine.asMaster().replicate(2);
		machines[0].sshKeys.push("key");
		deployment.deploy(machines);`,
		[]Machine{
			{
				Role:     "Master",
				Provider: "Amazon",
				SSHKeys:  []string{"key"},
			},
			{
				Role:     "Master",
				Provider: "Amazon",
				SSHKeys:  []string{},
			},
		},
	)
}

func TestContainer(t *testing.T) {
	t.Parallel()

	checkContainers(t, `deployment.deploy(new Label("foo", [
	new Container("image", ["arg1", "arg2"]).withEnv({"foo": "bar"})
	]));`,
		map[int]Container{
			1: {
				ID:      1,
				Image:   "image",
				Command: []string{"arg1", "arg2"},
				Env:     map[string]string{"foo": "bar"},
			},
		})

	checkContainers(t, `deployment.deploy(new Label("foo", [
	new Container("image", ["arg1", "arg2"])
	]));`,
		map[int]Container{
			1: {
				ID:      1,
				Image:   "image",
				Command: []string{"arg1", "arg2"},
				Env:     map[string]string{},
			},
		})

	checkContainers(t, `deployment.deploy(
		new Label("foo", [
		new Container("image")
		])
	);`,
		map[int]Container{
			1: {
				ID:      1,
				Image:   "image",
				Command: []string{},
				Env:     map[string]string{},
			},
		})

	checkContainers(t, `var c = new Container("image");
	c.env["foo"] = "bar";
	deployment.deploy(new Label("foo", [c]));`,
		map[int]Container{
			1: {
				ID:      1,
				Image:   "image",
				Command: []string{},
				Env:     map[string]string{"foo": "bar"},
			},
		})

	checkContainers(t, `deployment.deploy(
		new Label("foo", new Container("image", ["arg"]).replicate(2))
	);`,
		map[int]Container{
			// IDs start from 2 because the reference container has ID 1.
			2: {
				ID:      2,
				Image:   "image",
				Command: []string{"arg"},
				Env:     map[string]string{},
			},
			3: {
				ID:      3,
				Image:   "image",
				Command: []string{"arg"},
				Env:     map[string]string{},
			},
		})

	// Test changing attributes of replicated container.
	checkContainers(t, `var repl = new Container("image", ["arg"]).replicate(2);
	repl[0].env["foo"] = "bar";
	repl[0].command.push("changed");
	deployment.deploy(
		new Label("baz", repl)
	);`,
		map[int]Container{
			// IDs start from 2 because the reference container has ID 1.
			2: {
				ID:      2,
				Image:   "image",
				Command: []string{"arg", "changed"},
				Env: map[string]string{
					"foo": "bar",
				},
			},
			3: {
				ID:      3,
				Image:   "image",
				Command: []string{"arg"},
				Env:     map[string]string{},
			},
		})
}

func TestPlacement(t *testing.T) {
	t.Parallel()

	pre := `var target = new Label("target", []);
	var other = new Label("other", []);`
	post := `deployment.deploy(target);
	deployment.deploy(other);`
	checkPlacements(t, pre+`target.place(new LabelRule(true, other));`+post,
		[]Placement{
			{
				TargetLabel: "target",
				OtherLabel:  "other",
				Exclusive:   true,
			},
		})

	checkPlacements(t, pre+`target.place(new MachineRule(true,
	{size: "m4.large",
	region: "us-west-2",
	provider: "Amazon"}));`+post,
		[]Placement{
			{
				TargetLabel: "target",
				Exclusive:   true,
				Region:      "us-west-2",
				Provider:    "Amazon",
				Size:        "m4.large",
			},
		})

	checkPlacements(t, pre+`target.place(new MachineRule(true,
	{size: "m4.large",
	provider: "Amazon"}));`+post,
		[]Placement{
			{
				TargetLabel: "target",
				Exclusive:   true,
				Provider:    "Amazon",
				Size:        "m4.large",
			},
		})
}

func TestLabel(t *testing.T) {
	t.Parallel()

	checkLabels(t, `deployment.deploy(
		new Label("web_tier", [new Container("nginx")])
	);`,
		map[string]Label{
			"web_tier": {
				Name:        "web_tier",
				IDs:         []int{1},
				Annotations: []string{},
			},
		})

	checkLabels(t, `deployment.deploy(
		new Label("web_tier", [
		new Container("nginx"),
		new Container("nginx")
		])
	);`,
		map[string]Label{
			"web_tier": {
				Name:        "web_tier",
				IDs:         []int{1, 2},
				Annotations: []string{},
			},
		})

	// Conflicting label names.
	checkLabels(t, `deployment.deploy(new Label("foo", []));
	deployment.deploy(new Label("foo", []));`,
		map[string]Label{
			"foo": {
				Name:        "foo",
				IDs:         []int{},
				Annotations: []string{},
			},
			"foo2": {
				Name:        "foo2",
				IDs:         []int{},
				Annotations: []string{},
			},
		})

	expHostname := "foo.q"
	checkJavascript(t, `var foo = new Label("foo", []);
	return foo.hostname();`, expHostname)

	expChildren := []string{"1.foo.q", "2.foo.q"}
	checkJavascript(t, `
	var foo = new Label("foo",
	[new Container("bar"), new Container("baz")]);
	return foo.children();`, expChildren)
}

func TestConnect(t *testing.T) {
	t.Parallel()

	pre := `var foo = new Label("foo", []);
	var bar = new Label("bar", []);
	deployment.deploy([foo, bar]);`

	checkConnections(t, pre+`foo.connect(new Port(80), bar);`,
		[]Connection{
			{
				From:    "foo",
				To:      "bar",
				MinPort: 80,
				MaxPort: 80,
			},
		})

	checkConnections(t, pre+`foo.connect(new PortRange(80, 85), bar);`,
		[]Connection{
			{
				From:    "foo",
				To:      "bar",
				MinPort: 80,
				MaxPort: 85,
			},
		})

	checkConnections(t, pre+`foo.connectToPublic(new Port(80));`,
		[]Connection{
			{
				From:    "foo",
				To:      "public",
				MinPort: 80,
				MaxPort: 80,
			},
		})

	checkConnections(t, pre+`foo.connectToPublic(80);`,
		[]Connection{
			{
				From:    "foo",
				To:      "public",
				MinPort: 80,
				MaxPort: 80,
			},
		})

	checkConnections(t, pre+`foo.connectFromPublic(80);`,
		[]Connection{
			{
				From:    "public",
				To:      "foo",
				MinPort: 80,
				MaxPort: 80,
			},
		})

	checkError(t, pre+`foo.connectToPublic(new PortRange(80, 81));`,
		"public internet cannot connect on port ranges")
	checkError(t, pre+`foo.connectFromPublic(new PortRange(80, 81));`,
		"public internet cannot connect on port ranges")
}

func TestVet(t *testing.T) {
	pre := `var foo = new Label("foo", []);
	deployment.deploy([foo]);`

	// Connect to undeployed label.
	checkError(t, pre+`foo.connect(80, new Label("baz", []));`,
		"foo has a connection to undeployed label: baz")

	checkError(t, pre+`foo.place(new MachineRule(false, {
			provider: "Amazon"
		}));
	foo.place(new LabelRule(true, new Label("baz", [])));`,
		"foo has a placement in terms of an undeployed label: baz")
}

func TestCustomDeploy(t *testing.T) {
	t.Parallel()

	checkLabels(t, `deployment.deploy(
		{
			deploy: function(deployment) {
				deployment
				.deploy(new Label("web_tier", [new Container("nginx")]))
				.deploy(new Label("web_tier2", [new Container("nginx")]));
			}
		}
	);`,
		map[string]Label{
			"web_tier": {
				Name:        "web_tier",
				IDs:         []int{1},
				Annotations: []string{},
			},
			"web_tier2": {
				Name:        "web_tier2",
				IDs:         []int{2},
				Annotations: []string{},
			},
		})

	checkError(t, `deployment.deploy({})`, "can't deploy!")
}

func TestRequire(t *testing.T) {
	util.AppFs = afero.NewMemMapFs()

	// Import returning a primitive.
	util.WriteFile("math.js", []byte(`
	exports.square = function(x) {
		return x*x;
	};`), 0644)
	checkJavascript(t, `math = require('math');
	return math.square(5)`, float64(25))

	// Import returning a type.
	util.WriteFile("testImport.js", []byte(`
	exports.getLabel = function() {
		return new Label("foo", []);
	};`), 0644)
	checkJavascript(t, `testImport = require('testImport');
	return testImport.getLabel().hostname()`, "foo.q")

	// Import with an import
	util.WriteFile("square.js", []byte(`
	exports.square = function(x) {
		return x*x;
	};`), 0644)
	util.WriteFile("cube.js", []byte(`
	var square = require("square");
	exports.cube = function(x) {
		return x * square.square(x);
	};`), 0644)
	checkJavascript(t, `cube = require('cube');
	return cube.cube(5)`, float64(125))

	// Directly assigned exports
	util.WriteFile("square.js", []byte("module.exports = function(x) {"+
		"return x*x }"), 0644)
	checkJavascript(t, `var square = require('square');
	return square(5)`, float64(25))

	testSpec := `var square = require('square');
	square(5)`
	util.WriteFile("test.js", []byte(testSpec), 0644)
	compiled, err := Compile("test.js", ImportGetter{
		Path: ".",
	})
	if err != nil {
		t.Errorf(`Unexpected error: "%s".`, err.Error())
	}
	expCompiled := `importSources = {"square":"module.exports = ` +
		`function(x) {return x*x }"};` + testSpec
	if compiled != expCompiled {
		t.Errorf(`Bad compilation: expected "%s", got "%s".`,
			expCompiled, compiled)
	}
}

func TestGithubKeys(t *testing.T) {
	GetGithubKeys = func(username string) ([]string, error) {
		return []string{"keys"}, nil
	}
	checkJavascript(t, `return githubKeys("username");`, []string{"keys"})
}

func TestQuery(t *testing.T) {
	t.Parallel()

	namespaceChecker := queryChecker(func(handle Stitch) interface{} {
		return handle.QueryNamespace()
	})
	maxPriceChecker := queryChecker(func(handle Stitch) interface{} {
		return handle.QueryMaxPrice()
	})
	adminACLChecker := queryChecker(func(handle Stitch) interface{} {
		return handle.QueryAdminACL()
	})

	namespaceChecker(t, `createDeployment({namespace: "myNamespace"});`,
		"myNamespace")
	namespaceChecker(t, ``, "")
	maxPriceChecker(t, `createDeployment({maxPrice: 5});`, 5.0)
	maxPriceChecker(t, ``, 0.0)
	adminACLChecker(t, `createDeployment({adminACL: ["local"]});`, []string{"local"})
	adminACLChecker(t, ``, []string{})
}

func checkJavascript(t *testing.T, code string, exp interface{}) {
	resultKey := "result"

	exec := fmt.Sprintf(
		`var %s = function() {
			%s
		}()`, resultKey, code)
	getter := ImportGetter{
		Path: ".",
	}
	vm, err := run("<test_code>", exec, getter)
	if err != nil {
		t.Errorf(`Unexpected error: "%s".`, err.Error())
		return
	}

	actualVal, err := vm.Get(resultKey)
	if err != nil {
		t.Errorf(`Unexpected error retrieving result from VM: "%s".`,
			err.Error())
		return
	}

	actual, _ := actualVal.Export()
	if !reflect.DeepEqual(actual, exp) {
		t.Errorf("Bad javascript code: Expected %s, got %s.",
			spew.Sdump(exp), spew.Sdump(actual))
	}
}

func checkError(t *testing.T, code string, exp string) {
	_, err := New(code, DefaultImportGetter)
	if err == nil {
		t.Errorf(`Expected error "%s", but got nothing.`, exp)
		return
	}
	if actual := err.Error(); actual != exp {
		t.Errorf(`Expected error "%s", but got "%s".`, exp, actual)
	}
}

func queryChecker(
	queryFunc func(Stitch) interface{}) func(*testing.T, string, interface{}) {

	return func(t *testing.T, code string, exp interface{}) {
		handle, err := New(code, DefaultImportGetter)
		if err != nil {
			t.Errorf(`Unexpected error: "%s".`, err.Error())
			return
		}

		actual := queryFunc(handle)
		if !reflect.DeepEqual(actual, exp) {
			t.Errorf("Bad query: Expected %s, got %s.",
				spew.Sdump(exp), spew.Sdump(actual))
		}
	}
}

var checkMachines = queryChecker(func(s Stitch) interface{} {
	return s.QueryMachines()
})

var checkContainers = queryChecker(func(s Stitch) interface{} {
	// Convert the slice to a map because the ordering is non-deterministic.
	containersMap := make(map[int]Container)
	for _, c := range s.QueryContainers() {
		containersMap[c.ID] = c
	}
	return containersMap
})

var checkPlacements = queryChecker(func(s Stitch) interface{} {
	return s.QueryPlacements()
})

var checkLabels = queryChecker(func(s Stitch) interface{} {
	// Convert the slice to a map because the ordering is non-deterministic.
	labelsMap := make(map[string]Label)
	for _, label := range s.QueryLabels() {
		labelsMap[label.Name] = label
	}
	return labelsMap
})

var checkConnections = queryChecker(func(s Stitch) interface{} {
	return s.QueryConnections()
})
