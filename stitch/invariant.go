package stitch

import (
	"fmt"
	"strings"
)

type invariantType string

const (
	// Reachability (reach): two arguments, <from> <to...>
	reachInvariant = "reach"
	// Neighborship (reach-direct): two arguments, <from> <to>
	neighborInvariant = "reachDirect"
	// Reachability, don't pass through ACL-annotated nodes (reachACL):
	// two arguments, <from> <to...>
	reachACLInvariant = "reachACL"
	// On-pathness (between): three arguments, <from> <to> <between>
	betweenInvariant = "between"
	// Schedulability (enough): zero arguments
	schedulabilityInvariant = "enough"
)

// Annotations.
const (
	aclAnnotation = "ACL"
)

type invariantError struct {
	failer invariant
}

func (invErr invariantError) Error() string {
	return fmt.Sprintf("invariant failed: %s", invErr.failer)
}

// Even though `invariant` isn't exported, we have to export its fields so that
// we can unmarshal it with `encoding/json`.
type invariant struct {
	Form   invariantType
	Target bool     // Desired answer to invariant question.
	Nodes  []string // Nodes the invariant operates on.
}

func (inv invariant) String() string {
	tags := []string{string(inv.Form)}
	tags = append(tags, fmt.Sprintf("%t", inv.Target))
	for _, node := range inv.Nodes {
		tags = append(tags, fmt.Sprintf("%q", node))
	}
	return strings.Join(tags, " ")
}

var formImpls map[invariantType]func(graph Graph, inv invariant) bool

func init() {
	formImpls = map[invariantType]func(graph Graph, inv invariant) bool{
		reachInvariant:          reachImpl,
		neighborInvariant:       neighborImpl,
		reachACLInvariant:       reachACLImpl,
		betweenInvariant:        betweenImpl,
		schedulabilityInvariant: schedulabilityImpl,
	}
}

func checkInvariants(graph Graph, invs []invariant) error {
	for _, asrt := range invs {
		if val := formImpls[asrt.Form](graph, asrt); !val {
			return invariantError{asrt}
		}
	}

	return nil
}

func reachImpl(graph Graph, inv invariant) bool {
	var fromNodes []Node
	var toNodes []Node
	for _, node := range graph.Nodes {
		if node.Label == inv.Nodes[0] {
			fromNodes = append(fromNodes, node)
		}
		if node.Label == inv.Nodes[1] {
			toNodes = append(toNodes, node)
		}
	}

	for _, from := range fromNodes {
		for _, to := range toNodes {
			reachable := contains(from.dfs(), to.Name)
			if reachable != inv.Target {
				return false
			}
		}
	}

	return true
}

func neighborImpl(graph Graph, inv invariant) bool {
	var fromNodes []Node
	var toNodes []Node
	for _, node := range graph.Nodes {
		if node.Label == inv.Nodes[0] {
			fromNodes = append(fromNodes, node)
		}
		if node.Label == inv.Nodes[1] {
			toNodes = append(toNodes, node)
		}
	}

	for _, from := range fromNodes {
		for _, to := range toNodes {
			_, isNeighbor := from.Connections[to.Name]
			if isNeighbor != inv.Target {
				return false
			}
		}
	}

	return true
}

func reachACLImpl(graph Graph, inv invariant) bool {
	var fromNodes []Node
	var toNodes []Node
	for _, node := range graph.Nodes {
		if node.Label == inv.Nodes[0] {
			fromNodes = append(fromNodes, node)
		}
		if node.Label == inv.Nodes[1] {
			toNodes = append(toNodes, node)
		}
	}

	for _, from := range fromNodes {
		for _, to := range toNodes {
			if reachable := contains(from.dfsWithACL(),
				to.Name); reachable != inv.Target {
				return false
			}
		}
	}

	return true
}

func betweenImpl(graph Graph, inv invariant) bool {
	var fromNodes []Node
	var toNodes []Node
	var betweenNodes []Node
	for _, node := range graph.Nodes {
		switch node.Label {
		case inv.Nodes[0]:
			fromNodes = append(fromNodes, node)
		case inv.Nodes[1]:
			toNodes = append(toNodes, node)
		case inv.Nodes[2]:
			betweenNodes = append(betweenNodes, node)
		}
	}

	allPassed := true
	for _, from := range fromNodes {
		for _, to := range toNodes {
			allPassed = allPassed && betweenPathsHelper(
				betweenNodes,
				from,
				to,
				inv.Target,
			)
		}
	}
	return allPassed
}

func betweenPathsHelper(betweenNodes []Node, from Node, to Node, target bool) bool {
	paths, ok := paths(from, to)
	if !ok {
		// No path between source and dest.
		return !target
	}

	if target { // A betweenNode must be in all paths.
		allPaths := true
	pathsAll:
		for _, path := range paths {
			for _, between := range betweenNodes {
				if contains(path, between.Name) {
					break
				} else {
					allPaths = false
					break pathsAll
				}
			}
		}
		return allPaths
	}
	// A betweenNode must not be in any path.
	noPaths := true
pathsAny:
	for _, path := range paths {
		for _, between := range betweenNodes {
			if contains(path, between.Name) {
				noPaths = false
				break pathsAny
			}
		}
	}
	return noPaths
}

func schedulabilityImpl(graph Graph, inv invariant) bool {
	machines := graph.Machines
	avSets := graph.Availability
	if _, ok := graph.Nodes["public"]; ok {
		return len(machines) >= (len(avSets) - 1)
	}
	return len(machines) >= len(avSets)
}
