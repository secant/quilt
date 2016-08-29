package stitch

import (
	"testing"
)

func initSpec(src string) (Stitch, error) {
	return New(src, ImportGetter{
		Path: "../specs",
	})
}

func TestReach(t *testing.T) {
	stc := `var a = new Label("a", [new Container("ubuntu")]);
	var b = new Label("b", [new Container("ubuntu")]);
	var c = new Label("c", [new Container("ubuntu")]);
	a.connect(new Port(22), b);
	b.connect(new Port(22), c);

	deployment.deploy([a, b, c]);

	deployment.assert(a.canReach(c), true);
	deployment.assert(c.canReach(a), false);
	deployment.assert(c.between(a, b), true);
	deployment.assert(a.between(c, b), false);`
	_, err := initSpec(stc)
	if err != nil {
		t.Error(err)
	}
}

func TestReachPublic(t *testing.T) {
	stc := `var a = new Label("a", [new Container("ubuntu")]);
	var b = new Label("b", [new Container("ubuntu")]);
	var c = new Label("c", [new Container("ubuntu")]);
	a.connectToPublic(22);
	b.connectFromPublic(22);
	b.connect(22, c);

	deployment.deploy([a, b, c]);

	deployment.assert(b.canReachFromPublic(), true);
	deployment.assert(c.canReachFromPublic(), true);
	deployment.assert(a.canReachFromPublic(), false);
	deployment.assert(b.canReachPublic(), false);`
	_, err := initSpec(stc)
	if err != nil {
		t.Error(err)
	}
}

func TestNeighbor(t *testing.T) {
	stc := `var a = new Label("a", [new Container("ubuntu")]);
	var b = new Label("b", [new Container("ubuntu")]);
	var c = new Label("c", [new Container("ubuntu")]);
	a.connect(new Port(22), b);
	b.connect(new Port(22), c);

	deployment.deploy([a, b, c]);

	deployment.assert(a.neighborOf(c), false);
	deployment.assert(b.neighborOf(c), true);`
	_, err := initSpec(stc)
	if err != nil {
		t.Error(err)
	}
}

func TestAnnotation(t *testing.T) {
	stc := `var a = new Label("a", [new Container("ubuntu")]);
	var b = new Label("b", [new Container("ubuntu")]);
	var c = new Label("c", [new Container("ubuntu")]);
	a.connect(new Port(22), b);
	b.connect(new Port(22), c);

	b.annotate("ACL");

	deployment.assert(a.canReachACL(c), false);`

	_, err := initSpec(stc)
	if err != nil {
		t.Error(err)
	}
}

func TestFail(t *testing.T) {
	stc := `var a = new Label("a", [new Container("ubuntu")]);
	var b = new Label("b", [new Container("ubuntu")]);
	var c = new Label("c", [new Container("ubuntu")]);
	a.connect(new Port(22), b);
	b.connect(new Port(22), c);

	deployment.deploy([a, b, c]);

	deployment.assert(a.canReach(c), true);
	deployment.assert(c.canReach(a), true);`
	expectedFailure := `invariant failed: reach true "c" "a"`
	if _, err := initSpec(stc); err == nil {
		t.Errorf("got no error, expected %s", expectedFailure)
	} else if err.Error() != expectedFailure {
		t.Errorf("got error %s, expected %s", err, expectedFailure)
	}
}

func TestBetween(t *testing.T) {
	stc := `var a = new Label("a", [new Container("ubuntu")]);
	var b = new Label("b", [new Container("ubuntu")]);
	var c = new Label("c", [new Container("ubuntu")]);
	var d = new Label("d", [new Container("ubuntu")]);
	var e = new Label("e", [new Container("ubuntu")]);

	a.connect(new Port(22), b);
	a.connect(new Port(22), c);
	b.connect(new Port(22), d);
	c.connect(new Port(22), d);
	d.connect(new Port(22), e);

	deployment.deploy([a, b, c, d, e]);

	deployment.assert(a.canReach(e), true)
	deployment.assert(e.between(a, d), true)`
	_, err := initSpec(stc)
	if err != nil {
		t.Error(err)
	}
}

func TestNoConnect(t *testing.T) {
	t.Skip("wait for scheduler, use the new scheduling algorithm")
	stc := `(label "a" (docker "ubuntu"))
(label "b" (docker "ubuntu"))
(label "c" (docker "ubuntu"))
(label "d" (docker "ubuntu"))
(label "e" (docker "ubuntu"))

(let ((cfg (list (provider "Amazon")
                 (region "us-west-1")
                 (size "m4.2xlarge")
                 (diskSize 32))))
    (makeList 4 (machine (role "test") cfg)))

(place (labelRule "exclusive" "e") "b" "d")
(place (labelRule "exclusive" "c") "b" "d" "e")
(place (labelRule "exclusive" "a") "c" "d" "e")

(invariant enough)`
	_, err := initSpec(stc)
	if err != nil {
		t.Error(err)
	}
}

func TestNested(t *testing.T) {
	t.Skip("needs hierarchical labeling to pass")
	stc := `(label "a" (docker "ubuntu"))
(label "b" (docker "ubuntu"))
(label "c" (docker "ubuntu"))
(label "d" (docker "ubuntu"))

(label "g1" "a" "b")
(label "g2" "c" "d")

(connect 22 "g1" "g2")

(invariant reach true "a" "d")
(invariant reach true "b" "c")`
	_, err := initSpec(stc)
	if err != nil {
		t.Error(err)
	}
}

func TestPlacementInvs(t *testing.T) {
	t.Skip("wait for scheduler, use the new scheduling algorithm")
	stc := `(label "a" (docker "ubuntu"))
(label "b" (docker "ubuntu"))
(label "c" (docker "ubuntu"))
(label "d" (docker "ubuntu"))
(label "e" (docker "ubuntu"))

(connect 22 "a" "b")
(connect 22 "a" "c")
(connect 22 "b" "d")
(connect 22 "c" "d")
(connect 22 "d" "e")
(connect 22 "c" "e")

(let ((cfg (list (provider "Amazon")
                 (region "us-west-1")
                 (size "m4.2xlarge")
                 (diskSize 32))))
    (makeList 4 (machine (role "test") cfg)))

(place (labelRule "exclusive" "e") "b" "d")
(place (labelRule "exclusive" "c") "b" "d" "e")
(place (labelRule "exclusive" "a") "c" "d" "e")

(invariant reach true "a" "e")
(invariant enough)`
	_, err := initSpec(stc)
	if err != nil {
		t.Error(err)
	}
}
