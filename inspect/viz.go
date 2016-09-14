package inspect

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/NetSys/quilt/stitch"
)

func getSlug(configPath string) (string, error) {
	var slug string
	for i, ch := range configPath {
		if ch == '.' {
			slug = configPath[:i]
			break
		}
	}
	if len(slug) == 0 {
		return "", fmt.Errorf("could not find proper output file name")
	}

	return slug, nil
}

func viz(configPath string, spec stitch.Stitch, graph stitch.Graph, outputFormat string) {
	slug, err := getSlug(configPath)
	if err != nil {
		panic(err)
	}
	dot := makeGraphviz(graph)
	graphviz(outputFormat, slug, dot)
}

var wkClust = make(map[string]bool)
var msClust = make(map[string]bool)

func makeGraphviz(graph stitch.Graph) string {
	dotfile := "strict digraph {\n"
	subgraphs := make(map[string]string)
	for i, av := range graph.Availability {
		id, body := subGraph(i, graph, av.Nodes()...)
		subgraphs[id] = body
	}

	// Get masters first
	for k, v := range subgraphs {
		if msClust[k] {
			dotfile += v
			subgraphs[k] = ""
		}
	}

	// Put rest of subgraphs
	for _, v := range subgraphs {
		dotfile += v
	}

	var lines []string
	for _, edge := range graph.GetConnections() {
		efn := edge.From.Name
		efl := edge.From.Label
		etn := edge.To.Name
		etl := edge.To.Label
		dir := ""
		if strings.Contains(etl, "ms") {
			efn = edge.To.Name
			efl = edge.To.Label
			etn = edge.From.Name
			etl = edge.From.Label
			dir = `[dir="back"]`
		}
		lines = append(lines,
			fmt.Sprintf(
				"    {%s [label=%q]} -> {%s [label=%q]} %s\n",
				efn,
				efl,
				etn,
				etl,
				dir,
			),
		)
	}

	sort.Strings(lines)
	for _, line := range lines {
		dotfile += line + "\n"
	}

	dotfile += "}\n"

	return dotfile
}

func subGraph(i int, graph stitch.Graph, labels ...string) (string, string) {
	same := make(map[string][]string)
	nodes := graph.Nodes
	clust := fmt.Sprintf("cluster_%d", i)
	subgraph := fmt.Sprintf("    subgraph %s {\n", clust)
	str := ""
	sort.Strings(labels)
	for _, l := range labels {
		if l == stitch.PublicInternetLabel {
			return clust, ""
		}
		if strings.Contains(nodes[l].Label, "-ms") {
			msClust[clust] = true
		} else if strings.Contains(nodes[l].Label, "-wk") {
			wkClust[clust] = true
		}
		same[nodes[l].Label] = append(same[nodes[l].Label], l)
		str += l + "; "
	}

	sameRank := ""
	for k := range same {
		sameRank += genRank("same", same[k])
	}
	subgraph += "        " + str + "\n        color=white; \n" + sameRank + "    }\n"
	return clust, subgraph
}

func genRank(rank string, nodes []string) string {
	if len(nodes) == 0 {
		return ""
	}
	str := fmt.Sprintf("        {rank = %s;", rank)
	for _, l := range nodes {
		str += fmt.Sprintf(" %s", l)
	}
	str += "}\n"
	return str
}

// Graphviz generates a specification for the graphviz program that visualizes the
// communication graph of a stitch.
func graphviz(outputFormat string, slug string, dot string) {
	f, err := os.Create(slug + ".dot")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	defer func() {
		rm := exec.Command("rm", slug+".dot")
		rm.Run()
	}()
	fmt.Println(dot)
	f.Write([]byte(dot))

	// Dependencies:
	// - easy-graph (install Graph::Easy from cpan)
	// - graphviz (install from your favorite package manager)
	var writeGraph *exec.Cmd
	switch outputFormat {
	case "ascii":
		writeGraph = exec.Command("graph-easy", "--input="+slug+".dot",
			"--as_ascii")
	case "pdf":
		writeGraph = exec.Command("dot", "-Tpdf", "-o", slug+".pdf",
			slug+".dot")
	}
	writeGraph.Stdout = os.Stdout
	writeGraph.Stderr = os.Stderr
	writeGraph.Run()
}
