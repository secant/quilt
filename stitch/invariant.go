package stitch

import (
	"fmt"
)

type invariantType int

const (
	// Reachability (reach): two arguments, <from> <to...>
	reachInvariant = iota
	// Neighborship (reach-direct): two arguments, <from> <to>
	neighborInvariant
	// Reachability, don't pass through ACL-annotated nodes (reachACL):
	// two arguments, <from> <to...>
	reachACLInvariant
	// On-pathness (between): three arguments, <from> <to> <between>
	betweenInvariant
	// Schedulability (enough): zero arguments
	schedulabilityInvariant
)

// Annotations.
const (
	aclAnnotation = "ACL"
)

type invariant struct {
	form   invariantType
	target bool     // Desired answer to invariant question.
	nodes  []string // Nodes the invariant operates on.
	str    string   // Original invariant text.
}

func (i invariant) String() string {
	return i.str
}

func (i invariant) eval(ctx *evalCtx) (ast, error) {
	return i, nil
}

var formKeywords map[string]invariantType
var formImpls map[invariantType]func(graph Graph, inv invariant) bool

func init() {
	formKeywords = map[string]invariantType{
		"reach":       reachInvariant,
		"reachDirect": neighborInvariant,
		"reachACL":    reachACLInvariant,
		"between":     betweenInvariant,
		"enough":      schedulabilityInvariant,
	}

	formImpls = map[invariantType]func(graph Graph, inv invariant) bool{
		reachInvariant:          reachImpl,
		neighborInvariant:       neighborImpl,
		reachACLInvariant:       reachACLImpl,
		betweenInvariant:        betweenImpl,
		schedulabilityInvariant: schedulabilityImpl,
	}
}

func checkInvariants(graph Graph, invs []invariant) ([]invariant, *invariant, error) {
	for _, asrt := range invs {
		if val := formImpls[asrt.form](graph, asrt); !val {
			return invs, &asrt, fmt.Errorf("invariant failed")
		}
	}

	return invs, nil, nil
}

func reachImpl(graph Graph, inv invariant) bool {
	var fromNodes []Node
	var toNodes []Node
	for _, node := range graph.Nodes {
		if node.Label == inv.nodes[0] {
			fromNodes = append(fromNodes, node)
		}
		if node.Label == inv.nodes[1] {
			toNodes = append(toNodes, node)
		}
	}

	for _, from := range fromNodes {
		for _, to := range toNodes {
			reachable := contains(from.dfs(), to.Name)
			if reachable != inv.target {
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
		if node.Label == inv.nodes[0] {
			fromNodes = append(fromNodes, node)
		}
		if node.Label == inv.nodes[1] {
			toNodes = append(toNodes, node)
		}
	}

	for _, from := range fromNodes {
		for _, to := range toNodes {
			_, isNeighbor := from.Connections[to.Name]
			if isNeighbor != inv.target {
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
		if node.Label == inv.nodes[0] {
			fromNodes = append(fromNodes, node)
		}
		if node.Label == inv.nodes[1] {
			toNodes = append(toNodes, node)
		}
	}

	for _, from := range fromNodes {
		for _, to := range toNodes {
			if reachable := contains(from.dfsWithACL(),
				to.Name); reachable != inv.target {
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
		case inv.nodes[0]:
			fromNodes = append(fromNodes, node)
		case inv.nodes[1]:
			toNodes = append(toNodes, node)
		case inv.nodes[2]:
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
				inv.target,
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
