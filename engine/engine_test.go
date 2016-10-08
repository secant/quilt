package engine

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/NetSys/quilt/db"
	"github.com/NetSys/quilt/join"
	"github.com/NetSys/quilt/stitch"
	"github.com/davecgh/go-spew/spew"
)

func TestEngine(t *testing.T) {
	spew := spew.NewDefaultConfig()
	spew.MaxDepth = 2

	pre := `var deployment = createDeployment({
		namespace: "namespace",
		adminACL: ["1.2.3.4/32"],
	});
	var baseMachine = new Machine({provider: "Amazon", size: "m4.large"});`
	conn := db.New()

	code := pre + `deployment.deploy(baseMachine.asMaster().replicate(2))
		.deploy(baseMachine.asWorker().replicate(3))`

	UpdatePolicy(conn, prog(t, code))
	err := conn.Transact(func(view db.Database) error {
		cluster, err := view.GetCluster()
		masters := view.SelectFromMachine(func(m db.Machine) bool {
			return m.Role == db.Master
		})
		workers := view.SelectFromMachine(func(m db.Machine) bool {
			return m.Role == db.Worker
		})

		if err != nil {
			return err
		} else if len(cluster.AdminACLs) != 1 {
			return fmt.Errorf("bad cluster: %s", spew.Sdump(cluster))
		}

		if len(masters) != 2 {
			return fmt.Errorf("bad masters: %s", spew.Sdump(masters))
		}

		if len(workers) != 3 {
			return fmt.Errorf("bad workers: %s", spew.Sdump(workers))
		}
		return nil
	})
	if err != nil {
		t.Error(err.Error())
	}

	/* Verify master increase. */
	code = pre + `deployment.deploy(baseMachine.asMaster().replicate(4))
		.deploy(baseMachine.asWorker().replicate(5))`

	UpdatePolicy(conn, prog(t, code))
	err = conn.Transact(func(view db.Database) error {
		masters := view.SelectFromMachine(func(m db.Machine) bool {
			return m.Role == db.Master
		})
		workers := view.SelectFromMachine(func(m db.Machine) bool {
			return m.Role == db.Worker
		})

		if len(masters) != 4 {
			return fmt.Errorf("bad masters: %s", spew.Sdump(masters))
		}

		if len(workers) != 5 {
			return fmt.Errorf("bad workers: %s", spew.Sdump(workers))
		}
		return nil
	})
	if err != nil {
		t.Error(err.Error())
	}

	/* Verify that external writes stick around. */
	err = conn.Transact(func(view db.Database) error {
		masters := view.SelectFromMachine(func(m db.Machine) bool {
			return m.Role == db.Master
		})
		workers := view.SelectFromMachine(func(m db.Machine) bool {
			return m.Role == db.Worker
		})

		for _, master := range masters {
			master.CloudID = "1"
			master.PublicIP = "2"
			master.PrivateIP = "3"
			view.Commit(master)
		}

		for _, worker := range workers {
			worker.CloudID = "1"
			worker.PublicIP = "2"
			worker.PrivateIP = "3"
			view.Commit(worker)
		}

		return nil
	})

	/* Also verify that masters and workers decrease properly. */
	code = pre + `deployment.deploy(baseMachine.asMaster())
		.deploy(baseMachine.asWorker())`
	UpdatePolicy(conn, prog(t, code))
	err = conn.Transact(func(view db.Database) error {
		masters := view.SelectFromMachine(func(m db.Machine) bool {
			return m.Role == db.Master
		})
		workers := view.SelectFromMachine(func(m db.Machine) bool {
			return m.Role == db.Worker
		})

		if len(masters) != 1 || masters[0].CloudID != "1" ||
			masters[0].PublicIP != "2" || masters[0].PrivateIP != "3" {
			return fmt.Errorf("bad masters: %s", spew.Sdump(masters))
		}

		if len(workers) != 1 || workers[0].CloudID != "1" ||
			workers[0].PublicIP != "2" || workers[0].PrivateIP != "3" {
			return fmt.Errorf("bad workers: %s", spew.Sdump(workers))
		}
		return nil
	})
	if err != nil {
		t.Error(err.Error())
	}

	/* Empty Namespace does nothing. */
	code = pre + `deployment.namespace = "";
		deployment.deploy(baseMachine.asMaster())
			.deploy(baseMachine.asWorker())`
	UpdatePolicy(conn, prog(t, code))
	err = conn.Transact(func(view db.Database) error {
		masters := view.SelectFromMachine(func(m db.Machine) bool {
			return m.Role == db.Master
		})
		workers := view.SelectFromMachine(func(m db.Machine) bool {
			return m.Role == db.Worker
		})

		if len(masters) != 1 || masters[0].CloudID != "1" ||
			masters[0].PublicIP != "2" || masters[0].PrivateIP != "3" {
			return fmt.Errorf("bad masters: %s", spew.Sdump(masters))
		}

		if len(workers) != 1 || workers[0].CloudID != "1" ||
			workers[0].PublicIP != "2" || workers[0].PrivateIP != "3" {
			return fmt.Errorf("bad workers: %s", spew.Sdump(workers))
		}
		return nil
	})
	if err != nil {
		t.Error(err.Error())
	}

	/* Verify things go to zero. */
	code = pre + `deployment.deploy(baseMachine.asWorker())`
	UpdatePolicy(conn, prog(t, code))
	err = conn.Transact(func(view db.Database) error {
		masters := view.SelectFromMachine(func(m db.Machine) bool {
			return m.Role == db.Master
		})
		workers := view.SelectFromMachine(func(m db.Machine) bool {
			return m.Role == db.Worker
		})

		if len(masters) != 0 {
			return fmt.Errorf("bad masters: %s", spew.Sdump(masters))
		}

		if len(workers) != 0 {
			return fmt.Errorf("bad workers: %s", spew.Sdump(workers))
		}
		return nil
	})
	if err != nil {
		t.Error(err.Error())
	}

	// This function checks whether there is a one-to-one mapping for each machine
	// in `slice` to a provider in `providers`.
	providersInSlice := func(slice db.MachineSlice, providers db.ProviderSlice) bool {
		lKey := func(left interface{}) interface{} {
			return left.(db.Machine).Provider
		}
		rKey := func(right interface{}) interface{} {
			return right.(db.Provider)
		}
		_, l, r := join.HashJoin(slice, providers, lKey, rKey)
		return len(l) == 0 && len(r) == 0
	}

	/* Test mixed providers. */
	code = `deployment.deploy([
		new Machine({provider: "Amazon", size: "m4.large", role: "Master"}),
		new Machine({provider: "Vagrant", size: "v.large", role: "Master"}),
		new Machine({provider: "Azure", size: "a.large", role: "Worker"}),
		new Machine({provider: "Google", size: "g.large", role: "Worker"})]);`
	UpdatePolicy(conn, prog(t, code))
	err = conn.Transact(func(view db.Database) error {
		masters := view.SelectFromMachine(func(m db.Machine) bool {
			return m.Role == db.Master
		})
		workers := view.SelectFromMachine(func(m db.Machine) bool {
			return m.Role == db.Worker
		})

		if !providersInSlice(masters, db.ProviderSlice{db.Amazon, db.Vagrant}) {
			return fmt.Errorf("bad masters: %s", spew.Sdump(masters))
		}

		if !providersInSlice(workers, db.ProviderSlice{db.Azure, db.Google}) {
			return fmt.Errorf("bad workers: %s", spew.Sdump(workers))
		}
		return nil
	})
	if err != nil {
		t.Error(err.Error())
	}

	/* Test that machines with different providers don't match. */
	code = `deployment.deploy([
		new Machine({provider: "Amazon", size: "m4.large", role: "Master"}),
		new Machine({provider: "Azure", size: "a.large", role: "Master"}),
		new Machine({provider: "Amazon", size: "m4.large", role: "Worker"})]);`
	UpdatePolicy(conn, prog(t, code))
	err = conn.Transact(func(view db.Database) error {
		masters := view.SelectFromMachine(func(m db.Machine) bool {
			return m.Role == db.Master
		})

		if !providersInSlice(masters, db.ProviderSlice{db.Amazon, db.Azure}) {
			return fmt.Errorf("bad masters: %s", spew.Sdump(masters))
		}
		return nil
	})
	if err != nil {
		t.Error(err.Error())
	}
}

func TestContainer(t *testing.T) {
	spew := spew.NewDefaultConfig()
	spew.MaxDepth = 2
	conn := db.New()

	check := func(code string, red, blue, yellow int) error {
		UpdatePolicy(conn, prog(t, code))
		return conn.Transact(func(view db.Database) error {
			var redCount, blueCount, yellowCount int

			containers := view.SelectFromContainer(nil)
			for _, c := range containers {
				if len(c.Labels) != 1 {
					err := spew.Sprintf("two many labels: %s", c)
					return errors.New(err)
				}

				switch c.Labels[0] {
				case "Red":
					redCount++
				case "Blue":
					blueCount++
				case "Yellow":
					yellowCount++
				default:
					err := spew.Sprintf("unknown label: %s", c)
					return errors.New(err)
				}
			}

			if red != redCount || blue != blueCount ||
				yellow != yellowCount {
				return errors.New(
					spew.Sprintf("bad containers: %s", containers))
			}

			return nil
		})
	}

	code := `deployment.deploy([
		new Machine({provider: "Amazon", role: "Master"}),
		new Machine({provider: "Amazon", role: "Worker"}),
		new Label("Red", [new Container("alpine"), new Container("alpine")]),
		new Label("Blue", [new Container("alpine"), new Container("alpine")])
	]);`
	check(code, 2, 2, 0)

	code = `deployment.deploy([
		new Machine({provider: "Amazon", role: "Master"}),
		new Machine({provider: "Amazon", role: "Worker"}),
		new Label("Red", [new Container("alpine"),
			new Container("alpine"),
			new Container("alpine")]),
	]);`
	check(code, 3, 0, 0)

	code = `deployment.deploy([
		new Machine({provider: "Amazon", role: "Master"}),
		new Machine({provider: "Amazon", role: "Worker"}),
		new Label("Red", [new Container("alpine")]),
		new Label("Blue", new Container("alpine").replicate(5)),
		new Label("Yellow", new Container("alpine").replicate(10)),
	]);`
	check(code, 1, 5, 10)

	code = `deployment.deploy([
		new Machine({provider: "Amazon", role: "Master"}),
		new Machine({provider: "Amazon", role: "Worker"}),
		new Label("Red", new Container("alpine").replicate(30)),
		new Label("Blue", new Container("alpine").replicate(4)),
		new Label("Yellow", new Container("alpine").replicate(7)),
	]);`
	check(code, 30, 4, 7)

	code = `deployment.deploy([
		new Machine({provider: "Amazon", role: "Master"}),
		new Machine({provider: "Amazon", role: "Worker"}),
	]);`
	check(code, 0, 0, 0)
}

func TestSort(t *testing.T) {
	spew := spew.NewDefaultConfig()
	spew.MaxDepth = 2

	pre := `var baseMachine = new Machine({provider: "Amazon", size: "m4.large"});`
	conn := db.New()

	UpdatePolicy(conn, prog(t, pre+`deployment
		.deploy(baseMachine.asMaster().replicate(3))
		.deploy(baseMachine.asWorker().replicate(1))`))
	err := conn.Transact(func(view db.Database) error {
		machines := view.SelectFromMachine(func(m db.Machine) bool {
			return m.Role == db.Master
		})

		if len(machines) != 3 {
			return fmt.Errorf("bad machines: %s", spew.Sdump(machines))
		}

		machines[2].PublicIP = "a"
		machines[2].PrivateIP = "b"
		view.Commit(machines[2])

		machines[1].PrivateIP = "c"
		view.Commit(machines[1])

		return nil
	})
	if err != nil {
		t.Error(err.Error())
	}

	UpdatePolicy(conn, prog(t, pre+`deployment
		.deploy(baseMachine.asMaster().replicate(2))
		.deploy(baseMachine.asWorker().replicate(1))`))
	err = conn.Transact(func(view db.Database) error {
		machines := view.SelectFromMachine(func(m db.Machine) bool {
			return m.Role == db.Master
		})

		if len(machines) != 2 {
			return fmt.Errorf("bad machines: %s", spew.Sdump(machines))
		}

		for _, m := range machines {
			if m.PublicIP == "" && m.PrivateIP == "" {
				return fmt.Errorf("bad machine: %s",
					spew.Sdump(machines))
			}
		}

		return nil
	})
	if err != nil {
		t.Error(err.Error())
	}

	UpdatePolicy(conn, prog(t, pre+`deployment
		.deploy(baseMachine.asMaster().replicate(1))
		.deploy(baseMachine.asWorker().replicate(1))`))
	err = conn.Transact(func(view db.Database) error {
		machines := view.SelectFromMachine(func(m db.Machine) bool {
			return m.Role == db.Master
		})

		if len(machines) != 1 {
			return fmt.Errorf("bad machines: %s", spew.Sdump(machines))
		}

		for _, m := range machines {
			if m.PublicIP == "" || m.PrivateIP == "" {
				return fmt.Errorf("bad machine: %s",
					spew.Sdump(machines))
			}
		}

		return nil
	})
	if err != nil {
		t.Error(err.Error())
	}
}

func TestACLs(t *testing.T) {
	spew := spew.NewDefaultConfig()
	spew.MaxDepth = 2

	conn := db.New()

	code := `createDeployment({adminACL: ["1.2.3.4/32", "local"]})
		.deploy([
			new Machine({provider: "Amazon", role: "Master"}),
			new Machine({provider: "Amazon", role: "Worker"})
		]);`

	myIP = func() (string, error) {
		return "5.6.7.8", nil
	}
	UpdatePolicy(conn, prog(t, code))
	err := conn.Transact(func(view db.Database) error {
		cluster, err := view.GetCluster()

		if err != nil {
			return err
		}

		if !reflect.DeepEqual(cluster.AdminACLs,
			[]string{"1.2.3.4/32", "5.6.7.8/32"}) {
			return fmt.Errorf("bad ACLs: %s",
				spew.Sdump(cluster.AdminACLs))
		}

		return nil
	})
	if err != nil {
		t.Error(err.Error())
	}

	myIP = func() (string, error) {
		return "", errors.New("")
	}
	UpdatePolicy(conn, prog(t, code))
	err = conn.Transact(func(view db.Database) error {
		cluster, err := view.GetCluster()

		if err != nil {
			return err
		}

		if !reflect.DeepEqual(cluster.AdminACLs, []string{"1.2.3.4/32"}) {
			return fmt.Errorf("bad ACLs: %s",
				spew.Sdump(cluster.AdminACLs))
		}

		return nil
	})
	if err != nil {
		t.Error(err.Error())
	}
}

func prog(t *testing.T, code string) stitch.Stitch {
	result, err := stitch.New(code, stitch.DefaultImportGetter)
	if err != nil {
		t.Error(err.Error())
		return stitch.Stitch{}
	}

	return result
}
